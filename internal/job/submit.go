package job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Submitter submits raw blocks to the node RPC.
type Submitter struct {
	client *http.Client
	url    *url.URL
}

type submitReq struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type submitResp struct {
	Result interface{} `json:"result"`
	Error  interface{} `json:"error"`
}

// NewSubmitter builds a submitter using the same RPC URL format as RPCSource.
func NewSubmitter(rawURL string) (*Submitter, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse rpc url: %w", err)
	}
	return &Submitter{client: &http.Client{Timeout: 10 * time.Second}, url: parsed}, nil
}

// SubmitBlock submits a raw block hex via submitblock.
func (s *Submitter) SubmitBlock(ctx context.Context, blockHex string) error {
	body, err := json.Marshal(submitReq{JSONRPC: "1.0", ID: "juno-pool", Method: "submitblock", Params: []interface{}{blockHex}})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.url.User != nil {
		pw, _ := s.url.User.Password()
		req.SetBasicAuth(s.url.User.Username(), pw)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("submit status %d: %s", resp.StatusCode, string(data))
	}
	var rresp submitResp
	if err := json.Unmarshal(data, &rresp); err != nil {
		return fmt.Errorf("submit decode: %w", err)
	}
	if rresp.Error != nil {
		return fmt.Errorf("submit error: %v", rresp.Error)
	}
	return nil
}
