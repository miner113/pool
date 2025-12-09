package payout

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

// Wallet provides minimal RPC access for payouts using z_sendmany.
type Wallet struct {
	client      *http.Client
	url         *url.URL
	fromAddress string // shielded address to send from
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

// NewWallet builds a wallet RPC client from the node RPC URL.
// fromAddress should be a shielded/unified address that holds the pool's funds.
func NewWallet(rawURL string, fromAddress string) (*Wallet, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse rpc url: %w", err)
	}
	return &Wallet{
		client:      &http.Client{Timeout: 60 * time.Second},
		url:         parsed,
		fromAddress: fromAddress,
	}, nil
}

// SendToAddress uses z_sendmany to send from shielded pool to a transparent address.
// Returns the txid after the operation completes.
func (w *Wallet) SendToAddress(ctx context.Context, address string, amount float64) (string, error) {
	if address == "" {
		return "", fmt.Errorf("empty address")
	}
	if w.fromAddress == "" {
		return "", fmt.Errorf("no from address configured")
	}

	// z_sendmany format: z_sendmany "fromaddress" [{"address":"...", "amount":...}] minconf fee
	// Round amount to 8 decimal places for precision
	roundedAmount := float64(int64(amount*1e8)) / 1e8
	amounts := []map[string]interface{}{
		{"address": address, "amount": roundedAmount},
	}

	// Use minconf=1 and default fee (null)
	body, err := json.Marshal(rpcReq{
		JSONRPC: "1.0",
		ID:      "juno-pool",
		Method:  "z_sendmany",
		Params:  []interface{}{w.fromAddress, amounts, 1, nil},
	})
	if err != nil {
		return "", err
	}

	// Start the async operation
	opid, err := w.doRPC(ctx, body)
	if err != nil {
		return "", fmt.Errorf("z_sendmany: %w", err)
	}

	var operationID string
	if err := json.Unmarshal(opid, &operationID); err != nil {
		return "", fmt.Errorf("parse opid: %w", err)
	}

	// Poll for operation completion
	return w.waitForOperation(ctx, operationID)
}

// waitForOperation polls z_getoperationstatus until the operation completes.
func (w *Wallet) waitForOperation(ctx context.Context, opid string) (string, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			body, err := json.Marshal(rpcReq{
				JSONRPC: "1.0",
				ID:      "juno-pool",
				Method:  "z_getoperationstatus",
				Params:  []interface{}{[]string{opid}},
			})
			if err != nil {
				return "", err
			}

			result, err := w.doRPC(ctx, body)
			if err != nil {
				return "", err
			}

			var ops []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Result struct {
					TxID string `json:"txid"`
				} `json:"result"`
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(result, &ops); err != nil {
				return "", fmt.Errorf("parse operation status: %w", err)
			}

			if len(ops) == 0 {
				continue
			}

			op := ops[0]
			switch op.Status {
			case "success":
				return op.Result.TxID, nil
			case "failed":
				return "", fmt.Errorf("operation failed: %s", op.Error.Message)
			case "executing", "queued":
				// Still running, continue polling
			default:
				return "", fmt.Errorf("unknown operation status: %s", op.Status)
			}
		}
	}
}

// doRPC executes an RPC call and returns the result.
func (w *Wallet) doRPC(ctx context.Context, body []byte) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.url.User != nil {
		pw, _ := w.url.User.Password()
		req.SetBasicAuth(w.url.User.Username(), pw)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wallet status %d: %s", resp.StatusCode, string(data))
	}
	var rresp rpcResp
	if err := json.Unmarshal(data, &rresp); err != nil {
		return nil, fmt.Errorf("wallet decode: %w", err)
	}
	if rresp.Error != nil {
		return nil, fmt.Errorf("wallet error: %v", rresp.Error)
	}
	return rresp.Result, nil
}

// ShieldCoinbase shields all coinbase UTXOs to the pool's shielded address.
// This must be called before payouts since coinbase can't be sent directly to transparent addresses.
// Returns the number of UTXOs shielded and any error.
func (w *Wallet) ShieldCoinbase(ctx context.Context) (int, error) {
	if w.fromAddress == "" {
		return 0, fmt.Errorf("no from address configured")
	}

	body, err := json.Marshal(rpcReq{
		JSONRPC: "1.0",
		ID:      "juno-pool",
		Method:  "z_shieldcoinbase",
		Params:  []interface{}{"*", w.fromAddress},
	})
	if err != nil {
		return 0, err
	}

	result, err := w.doRPC(ctx, body)
	if err != nil {
		// "No coinbase UTXOs to shield" is not an error
		return 0, nil
	}

	var shieldResult struct {
		RemainingUTXOs int     `json:"remainingUTXOs"`
		ShieldingUTXOs int     `json:"shieldingUTXOs"`
		ShieldingValue float64 `json:"shieldingValue"`
		OpID           string  `json:"opid"`
	}
	if err := json.Unmarshal(result, &shieldResult); err != nil {
		return 0, fmt.Errorf("parse shield result: %w", err)
	}

	if shieldResult.ShieldingUTXOs == 0 {
		return 0, nil
	}

	// Wait for the shielding operation to complete
	_, err = w.waitForOperation(ctx, shieldResult.OpID)
	if err != nil {
		return 0, fmt.Errorf("shield operation: %w", err)
	}

	return shieldResult.ShieldingUTXOs, nil
}
