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

// Template captures the fields needed to build stratum notify messages.
// RPCSource implements Source against the Juno RPC (getblocktemplate).
type RPCSource struct {
	client *http.Client
	url    *url.URL
}

// NewRPCSource creates a Source using the given RPC URL (may contain userinfo).
func NewRPCSource(rawURL string) (*RPCSource, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse rpc url: %w", err)
	}
	return &RPCSource{client: &http.Client{Timeout: 10 * time.Second}, url: parsed}, nil
}

type rpcReq struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcResp struct {
	Result templateResult `json:"result"`
	Error  interface{}    `json:"error"`
}

type templateResult struct {
	Version           int64            `json:"version"`
	PreviousBlockhash string           `json:"previousblockhash"`
	Bits              string           `json:"bits"`
	Target            string           `json:"target"`
	Mintime           int64            `json:"mintime"`
	Curtime           int64            `json:"curtime"`
	Height            int64            `json:"height"`
	NonceRange        string           `json:"noncerange"`
	CoinbaseTxn       templateCoinbase `json:"coinbasetxn"`
	Transactions      []templateTx     `json:"transactions"`
	DefaultRoots      defaultRoots     `json:"defaultroots"`
	RandomXSeedHash   string           `json:"randomxseedhash"`
	RandomXSeedHeight int64            `json:"randomxseedheight"`
}

type defaultRoots struct {
	MerkleRoot           string `json:"merkleroot"`
	ChainHistoryRoot     string `json:"chainhistoryroot"`
	AuthDataRoot         string `json:"authdataroot"`
	BlockCommitmentsHash string `json:"blockcommitmentshash"`
}

type templateCoinbase struct {
	Data string `json:"data"`
	Hash string `json:"hash"` // ZIP-244 txid
}

type templateTx struct {
	Hash string `json:"hash"`
	Data string `json:"data"`
}

// Next returns a fresh block template.
func (r *RPCSource) Next(ctx context.Context) (*Template, error) {
	body, err := json.Marshal(rpcReq{JSONRPC: "1.0", ID: "juno-pool", Method: "getblocktemplate", Params: []interface{}{}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.url.User != nil {
		pw, _ := r.url.User.Password()
		req.SetBasicAuth(r.url.User.Username(), pw)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc status %d: %s", resp.StatusCode, string(data))
	}
	var rresp rpcResp
	if err := json.Unmarshal(data, &rresp); err != nil {
		return nil, fmt.Errorf("rpc decode: %w", err)
	}
	tr := rresp.Result
	var txHashes []string
	var txData []string
	for _, tx := range tr.Transactions {
		txHashes = append(txHashes, tx.Hash)
		txData = append(txData, tx.Data)
	}
	return &Template{
		JobID:          fmt.Sprintf("%d-%d", tr.Height, tr.Curtime),
		PrevHash:       tr.PreviousBlockhash,
		Bits:           tr.Bits,
		Version:        tr.Version,
		Target:         tr.Target,
		Height:         tr.Height,
		CurTime:        tr.Curtime,
		Mintime:        tr.Mintime,
		NonceRange:     tr.NonceRange,
		CoinbaseTxn:    tr.CoinbaseTxn.Data,
		CoinbaseHash:   tr.CoinbaseTxn.Hash,
		MerkleRoot:     tr.DefaultRoots.MerkleRoot,
		MerkleBranches: BuildMerkleBranches(txHashes),
		BlockCommit:    tr.DefaultRoots.BlockCommitmentsHash,
		RandomXSeed:    tr.RandomXSeedHash,
		RandomXHeight:  tr.RandomXSeedHeight,
		Transactions:   txData,
	}, nil
}
