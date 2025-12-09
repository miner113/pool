package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"juno-pool/internal/db"
	"juno-pool/internal/metrics"
	"juno-pool/internal/network"
)

// StatsProvider provides live stats from the stratum server.
type StatsProvider interface {
	GetSessionStats() (count int, hashrate float64)
}

// PayoutRunner allows triggering manual payouts.
type PayoutRunner interface {
	RunNow() error
}

// Server provides the REST API for the pool dashboard.
type Server struct {
	store          *db.Store
	metrics        *metrics.PromRecorder
	mux            *http.ServeMux
	networkFetcher *network.Fetcher

	// In-memory stats updated by stratum
	mu             sync.RWMutex
	connectedCount int
	totalHashrate  float64
	minerHashrates map[string]float64
	poolStats      PoolStats

	// Stats provider (stratum server)
	statsProvider StatsProvider

	// Payout runner for manual payout trigger
	payoutRunner PayoutRunner
}

// PoolStats holds aggregate pool statistics.
type PoolStats struct {
	ConnectedMiners   int       `json:"connected_miners"`
	TotalHashrate     float64   `json:"total_hashrate"`
	BlocksFound       int64     `json:"blocks_found"`
	BlocksConfirmed   int64     `json:"blocks_confirmed"`
	LastBlockTime     time.Time `json:"last_block_time,omitempty"`
	TotalSharesDay    int64     `json:"total_shares_24h"`
	NetworkDifficulty float64   `json:"network_difficulty"`
	PoolFee           float64   `json:"pool_fee_percent"`
}

// MinerStats holds individual miner statistics.
type MinerStats struct {
	Username       string    `json:"username"`
	Hashrate       float64   `json:"hashrate"`
	SharesAccepted int64     `json:"shares_accepted"`
	SharesRejected int64     `json:"shares_rejected"`
	LastShare      time.Time `json:"last_share,omitempty"`
	Balance        float64   `json:"balance"`
	TotalPaid      float64   `json:"total_paid"`
	PayoutAddress  string    `json:"payout_address,omitempty"`
}

// BlockInfo represents a block for API responses.
type BlockInfo struct {
	Height        int64     `json:"height"`
	Hash          string    `json:"hash"`
	Status        string    `json:"status"`
	Confirmations int       `json:"confirmations"`
	Reward        float64   `json:"reward,omitempty"`
	FoundAt       time.Time `json:"found_at"`
}

// PaymentInfo represents a payment for API responses.
type PaymentInfo struct {
	TxID      string    `json:"txid"`
	Amount    float64   `json:"amount"`
	Status    string    `json:"status"`
	Address   string    `json:"address,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// New creates a new API server.
func New(store *db.Store, prom *metrics.PromRecorder, poolFee float64, networkFetcher *network.Fetcher) *Server {
	s := &Server{
		store:          store,
		metrics:        prom,
		mux:            http.NewServeMux(),
		minerHashrates: make(map[string]float64),
		networkFetcher: networkFetcher,
		poolStats: PoolStats{
			PoolFee: poolFee,
		},
	}
	s.registerRoutes()
	return s
}

// SetStatsProvider sets the stratum server as a stats provider.
func (s *Server) SetStatsProvider(sp StatsProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statsProvider = sp
}

// SetPayoutRunner sets the payout service for manual payout trigger.
func (s *Server) SetPayoutRunner(pr PayoutRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payoutRunner = pr
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/pool/stats", s.corsMiddleware(s.handlePoolStats))
	s.mux.HandleFunc("/api/pool/blocks", s.corsMiddleware(s.handleBlocks))
	s.mux.HandleFunc("/api/pool/payments", s.corsMiddleware(s.handlePayments))
	s.mux.HandleFunc("/api/pool/miners", s.corsMiddleware(s.handleMiners))
	s.mux.HandleFunc("/api/network/stats", s.corsMiddleware(s.handleNetworkStats))
	s.mux.HandleFunc("/api/miner/", s.corsMiddleware(s.handleMinerStats))
	s.mux.HandleFunc("/api/miner/lookup", s.corsMiddleware(s.handleMinerLookup))
	s.mux.HandleFunc("/api/admin/payout", s.corsMiddleware(s.handleManualPayout))
}

func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// Handler returns the HTTP handler for the API.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// UpdateMinerHashrate updates the in-memory hashrate for a miner.
func (s *Server) UpdateMinerHashrate(username string, hashrate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.minerHashrates[username] = hashrate

	// Recalculate total
	total := 0.0
	for _, h := range s.minerHashrates {
		total += h
	}
	s.totalHashrate = total
}

// UpdateConnectedCount updates the connected miner count.
func (s *Server) UpdateConnectedCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectedCount = count
}

func (s *Server) handlePoolStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	s.mu.RLock()
	stats := PoolStats{
		ConnectedMiners: s.connectedCount,
		TotalHashrate:   s.totalHashrate,
		PoolFee:         s.poolStats.PoolFee,
	}
	sp := s.statsProvider
	s.mu.RUnlock()

	// Get live stats from stratum server if available
	if sp != nil {
		count, hashrate := sp.GetSessionStats()
		stats.ConnectedMiners = count
		stats.TotalHashrate = hashrate
	}

	// Get block counts from DB
	if s.store != nil {
		blocks, err := s.store.RecentBlocks(ctx, 1000)
		if err == nil {
			stats.BlocksFound = int64(len(blocks))
			for _, b := range blocks {
				if b.Status == "confirmed" {
					stats.BlocksConfirmed++
				}
			}
			if len(blocks) > 0 {
				stats.LastBlockTime = blocks[0].CreatedAt
			}
		}

		// Get 24h share count
		shares24h, err := s.store.ShareCount24h(ctx)
		if err == nil {
			stats.TotalSharesDay = shares24h
		}
	}

	s.writeJSON(w, stats)
}

func (s *Server) handleNetworkStats(w http.ResponseWriter, r *http.Request) {
	if s.networkFetcher == nil {
		s.writeJSON(w, map[string]any{
			"block_height":     0,
			"difficulty":       0,
			"network_hashrate": 0,
			"chain":            "unknown",
		})
		return
	}
	stats := s.networkFetcher.Get()
	s.writeJSON(w, stats)
}

func (s *Server) handleBlocks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	var blocks []BlockInfo
	if s.store != nil {
		rows, err := s.store.RecentBlocks(ctx, limit)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		for _, b := range rows {
			blocks = append(blocks, BlockInfo{
				Height:        b.Height,
				Hash:          b.Hash,
				Status:        b.Status,
				Confirmations: b.Confirms,
				FoundAt:       b.CreatedAt,
			})
		}
	}

	s.writeJSON(w, map[string]any{
		"blocks": blocks,
		"count":  len(blocks),
	})
}

func (s *Server) handlePayments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	var payments []PaymentInfo
	if s.store != nil {
		rows, err := s.store.RecentPayouts(ctx, limit)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		for _, p := range rows {
			payments = append(payments, PaymentInfo{
				TxID:      p.TxID,
				Amount:    p.Amount,
				Status:    p.Status,
				Timestamp: p.Created,
			})
		}
	}

	s.writeJSON(w, map[string]any{
		"payments": payments,
		"count":    len(payments),
	})
}

func (s *Server) handleMinerStats(w http.ResponseWriter, r *http.Request) {
	// Extract username from URL path: /api/miner/{username}
	path := strings.TrimPrefix(r.URL.Path, "/api/miner/")
	username := strings.Split(path, "/")[0]
	if username == "" || username == "lookup" {
		s.writeError(w, http.StatusBadRequest, "miner address required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats := MinerStats{
		Username: username,
	}

	// Get hashrate from memory
	s.mu.RLock()
	stats.Hashrate = s.minerHashrates[username]
	s.mu.RUnlock()

	// Get stats from DB
	if s.store != nil {
		minerStats, err := s.store.GetMinerStats(ctx, username)
		if err == nil {
			stats.SharesAccepted = minerStats.SharesAccepted
			stats.SharesRejected = minerStats.SharesRejected
			stats.LastShare = minerStats.LastShare
			stats.Balance = minerStats.Balance
			stats.TotalPaid = minerStats.TotalPaid
			stats.PayoutAddress = minerStats.PayoutAddress
		}
	}

	s.writeJSON(w, stats)
}

func (s *Server) handleMinerLookup(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		s.writeError(w, http.StatusBadRequest, "address parameter required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if s.store != nil {
		exists, err := s.store.MinerExists(ctx, address)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		s.writeJSON(w, map[string]any{
			"address": address,
			"exists":  exists,
		})
		return
	}

	s.writeJSON(w, map[string]any{
		"address": address,
		"exists":  false,
	})
}

func (s *Server) handleMiners(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type MinerSummary struct {
		Username string  `json:"username"`
		Hashrate float64 `json:"hashrate"`
		Balance  float64 `json:"balance"`
	}

	var miners []MinerSummary

	if s.store != nil {
		balances, err := s.store.GetAllBalances(ctx)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		s.mu.RLock()
		for _, b := range balances {
			miners = append(miners, MinerSummary{
				Username: b.Username,
				Hashrate: s.minerHashrates[b.Username],
				Balance:  b.Balance,
			})
		}
		s.mu.RUnlock()
	}

	s.writeJSON(w, map[string]any{
		"miners": miners,
		"count":  len(miners),
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("api: json encode error: %v", err)
	}
}

func (s *Server) handleManualPayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	s.mu.RLock()
	pr := s.payoutRunner
	s.mu.RUnlock()

	if pr == nil {
		s.writeError(w, http.StatusServiceUnavailable, "payout service not available")
		return
	}

	log.Println("api: manual payout triggered")
	if err := pr.RunNow(); err != nil {
		log.Printf("api: manual payout error: %v", err)
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]any{
		"success": true,
		"message": "payout executed successfully",
	})
}

func (s *Server) writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
