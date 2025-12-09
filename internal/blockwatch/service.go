package blockwatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"juno-pool/internal/config"
	"juno-pool/internal/db"
)

// Service polls the node for submitted blocks to mark confirmed/orphan.
type Service struct {
	store    *db.Store
	client   *http.Client
	url      *url.URL
	confirm  int
	interval time.Duration
}

// New builds a block watcher using node RPC URL.
func New(store *db.Store, cfg config.Config) (*Service, error) {
	parsed, err := url.Parse(cfg.NodeRPCURL)
	if err != nil {
		return nil, fmt.Errorf("parse rpc url: %w", err)
	}
	return &Service{
		store:    store,
		client:   &http.Client{Timeout: 10 * time.Second},
		url:      parsed,
		confirm:  cfg.BlockConfirmations,
		interval: 30 * time.Second,
	}, nil
}

type rpcReq struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type headerResp struct {
	Result *struct {
		Confirmations int `json:"confirmations"`
	} `json:"result"`
	Error interface{} `json:"error"`
}

// Start begins polling; returns stop function.
func (s *Service) Start() func() {
	if s.store == nil {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.checkOnce()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

func (s *Service) checkOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blocks, err := s.store.PendingBlocks(ctx, 50)
	if err != nil {
		return
	}
	for _, b := range blocks {
		if b.Hash == "" {
			continue
		}
		confs, err := s.fetchConfirmations(ctx, b.Hash)
		if err != nil {
			continue
		}
		status := b.Status
		if confs >= s.confirm {
			status = "confirmed"
		} else if confs < 0 {
			status = "orphan"
		}
		s.store.UpdateBlockConfirmations(ctx, b.Hash, confs, status)
	}
}

func (s *Service) fetchConfirmations(ctx context.Context, hash string) (int, error) {
	body, err := json.Marshal(rpcReq{JSONRPC: "1.0", ID: "juno-pool", Method: "getblockheader", Params: []interface{}{hash}})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url.String(), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.url.User != nil {
		pw, _ := s.url.User.Password()
		req.SetBasicAuth(s.url.User.Username(), pw)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("rpc status %d", resp.StatusCode)
	}
	var h headerResp
	if err := json.Unmarshal(data, &h); err != nil {
		return 0, err
	}
	if h.Error != nil {
		return 0, fmt.Errorf("rpc error: %v", h.Error)
	}
	if h.Result == nil {
		return 0, fmt.Errorf("no result")
	}
	return h.Result.Confirmations, nil
}
