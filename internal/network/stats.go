package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Stats holds network statistics from the node.
type Stats struct {
	BlockHeight     int64   `json:"block_height"`
	Difficulty      float64 `json:"difficulty"`
	NetworkHashrate float64 `json:"network_hashrate"`
	Chain           string  `json:"chain"`
	Blocks          int64   `json:"blocks"`
	Headers         int64   `json:"headers"`
	BestBlockHash   string  `json:"best_block_hash"`
}

// Fetcher periodically fetches network stats from the node RPC.
type Fetcher struct {
	client *http.Client
	url    *url.URL

	mu    sync.RWMutex
	stats Stats
}

// NewFetcher creates a network stats fetcher.
func NewFetcher(rawURL string) (*Fetcher, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse rpc url: %w", err)
	}
	return &Fetcher{
		client: &http.Client{Timeout: 10 * time.Second},
		url:    parsed,
	}, nil
}

type rpcReq struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcResp struct {
	Result json.RawMessage `json:"result"`
	Error  interface{}     `json:"error"`
}

// Get returns the current cached network stats.
func (f *Fetcher) Get() Stats {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stats
}

// Fetch retrieves fresh network stats from the node.
func (f *Fetcher) Fetch(ctx context.Context) error {
	// Get blockchain info
	bcInfo, err := f.callRPC(ctx, "getblockchaininfo", nil)
	if err != nil {
		return fmt.Errorf("getblockchaininfo: %w", err)
	}

	var chainInfo struct {
		Chain         string  `json:"chain"`
		Blocks        int64   `json:"blocks"`
		Headers       int64   `json:"headers"`
		BestBlockHash string  `json:"bestblockhash"`
		Difficulty    float64 `json:"difficulty"`
	}
	if err := json.Unmarshal(bcInfo, &chainInfo); err != nil {
		return fmt.Errorf("parse blockchain info: %w", err)
	}

	// Get network hashrate (getnetworkhashps returns H/s)
	hashpsResult, err := f.callRPC(ctx, "getnetworkhashps", []interface{}{120}) // 120 blocks average
	if err != nil {
		// Some nodes might not support this, continue with 0
		hashpsResult = []byte("0")
	}

	var networkHashrate float64
	json.Unmarshal(hashpsResult, &networkHashrate)

	f.mu.Lock()
	f.stats = Stats{
		BlockHeight:     chainInfo.Blocks,
		Difficulty:      chainInfo.Difficulty,
		NetworkHashrate: networkHashrate,
		Chain:           chainInfo.Chain,
		Blocks:          chainInfo.Blocks,
		Headers:         chainInfo.Headers,
		BestBlockHash:   chainInfo.BestBlockHash,
	}
	f.mu.Unlock()

	return nil
}

// Start begins periodic fetching of network stats.
func (f *Fetcher) Start(interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})

	// Fetch immediately
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	f.Fetch(ctx)
	cancel()

	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := f.Fetch(ctx); err != nil {
					// Log but don't stop
				}
				cancel()
			}
		}
	}()

	return func() { close(done) }
}

func (f *Fetcher) callRPC(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	if params == nil {
		params = []interface{}{}
	}
	body, err := json.Marshal(rpcReq{
		JSONRPC: "1.0",
		ID:      "network-stats",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", f.url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if f.url.User != nil {
		pass, _ := f.url.User.Password()
		req.SetBasicAuth(f.url.User.Username(), pass)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("rpc status %d: %s", resp.StatusCode, string(data))
	}

	var rresp rpcResp
	if err := json.Unmarshal(data, &rresp); err != nil {
		return nil, err
	}
	if rresp.Error != nil {
		return nil, fmt.Errorf("rpc error: %v", rresp.Error)
	}

	return rresp.Result, nil
}
