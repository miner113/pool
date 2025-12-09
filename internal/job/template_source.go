package job

import "context"

// Template carries the fields needed to build stratum notify messages.
type Template struct {
	JobID          string
	PrevHash       string
	Bits           string
	Version        int64
	Target         string
	Height         int64
	CurTime        int64
	Mintime        int64
	NonceRange     string
	CoinbaseTxn    string
	CoinbaseHash   string // Txid of coinbase (from getblocktemplate, ZIP-244 computed)
	MerkleRoot     string // Pre-computed merkle root from defaultroots
	MerkleBranches []string
	BlockCommit    string
	RandomXSeed    string
	RandomXHeight  int64
	RawJSON        string
	Transactions   []string
}

// Source fetches block templates.
type Source interface {
	Next(ctx context.Context) (*Template, error)
}
