package payout

import (
	"context"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"juno-pool/internal/db"
)

const satoshisPerCoin = 100000000.0 // 1 JUNO = 100,000,000 satoshis

// Service periodically scans balances and schedules payouts once above threshold.
type Service struct {
	store     *db.Store
	wallet    *Wallet
	threshold float64 // threshold in JUNO
	cronSpec  string
	stopCh    chan struct{}
}

// New constructs a payout service.
func New(store *db.Store, wallet *Wallet, threshold float64, cronSpec string) *Service {
	return &Service{store: store, wallet: wallet, threshold: threshold, cronSpec: cronSpec, stopCh: make(chan struct{})}
}

// Start registers the cron job. It returns a function to stop the scheduler.
func (s *Service) Start() (func(), error) {
	c := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	_, err := c.AddFunc(s.cronSpec, s.run)
	if err != nil {
		return nil, err
	}
	c.Start()
	return func() {
		close(s.stopCh)
		c.Stop()
	}, nil
}

func (s *Service) run() {
	_ = s.RunNow()
}

// RunNow executes payout immediately (for manual trigger or testing).
func (s *Service) RunNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Longer timeout for shielding
	defer cancel()

	// Step 1: Auto-shield any coinbase UTXOs to the pool's shielded address
	shielded, err := s.wallet.ShieldCoinbase(ctx)
	if err != nil {
		log.Printf("payout: shield coinbase failed: %v", err)
		// Continue anyway - there might be already-shielded funds
	} else if shielded > 0 {
		log.Printf("payout: shielded %d coinbase UTXOs", shielded)
	}

	// Convert threshold from JUNO to satoshis for DB query (balance is stored in satoshis)
	thresholdSatoshis := s.threshold * satoshisPerCoin

	rows, err := s.store.BalancesAbove(ctx, thresholdSatoshis)
	if err != nil {
		log.Printf("payout sweep balances: %v", err)
		return err
	}
	for _, r := range rows {
		amountSatoshis := r.Balance
		if amountSatoshis <= 0 {
			continue
		}
		addr := r.Payout
		if addr == "" {
			log.Printf("payout skipped user=%s no payout address", r.Username)
			continue
		}
		// Convert satoshis to JUNO for the RPC call
		amountJuno := amountSatoshis / satoshisPerCoin
		log.Printf("payout attempting user=%s amount=%.8f JUNO (%.0f satoshis) to %s", r.Username, amountJuno, amountSatoshis, addr)

		txid, err := s.wallet.SendToAddress(ctx, addr, amountJuno)
		if err != nil {
			log.Printf("payout send failed user=%s amount=%.8f JUNO: %v", r.Username, amountJuno, err)
			s.store.RecordPayout(ctx, r.Username, amountSatoshis, "failed", "")
			continue
		}
		if err := s.store.DeductAndRecordPayout(ctx, r.Username, amountSatoshis, "sent", txid); err != nil {
			log.Printf("payout record failed for %s: %v", r.Username, err)
			continue
		}
		log.Printf("payout sent user=%s amount=%.8f JUNO txid=%s", r.Username, amountJuno, txid)

		// Wait briefly between transactions to avoid duplicate nullifier errors
		// The wallet needs time to update its internal state after each tx
		time.Sleep(2 * time.Second)
	}
	return nil
}
