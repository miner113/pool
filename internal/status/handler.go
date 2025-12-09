package status

import (
	"encoding/json"
	"net/http"
	"time"

	"juno-pool/internal/db"
)

// Handler serves a lightweight JSON status page with recent blocks and shares.
type Handler struct {
	store *db.Store
	limit int
}

// New returns a status handler using the given store. Limit controls how many records to show.
func New(store *db.Store, limit int) http.Handler {
	if limit <= 0 {
		limit = 20
	}
	return &Handler{store: store, limit: limit}
}

type response struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Blocks      []db.BlockRow `json:"blocks"`
	Shares      []db.ShareRow `json:"shares"`
	Payouts     []struct {
		Username string
		Amount   float64
		Status   string
		TxID     string
		Created  time.Time
		Updated  time.Time
	} `json:"payouts"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	blocks, _ := h.store.RecentBlocks(ctx, h.limit)
	shares, _ := h.store.RecentShares(ctx, h.limit)
	payouts, _ := h.store.RecentPayouts(ctx, h.limit)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{
		GeneratedAt: time.Now().UTC(),
		Blocks:      blocks,
		Shares:      shares,
		Payouts:     payouts,
	})
}
