package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PromRecorder implements Recorder backed by Prometheus counters/gauges.
type PromRecorder struct {
	registry        *prometheus.Registry
	handler         http.Handler
	connOpened      prometheus.Counter
	connClosed      prometheus.Counter
	shareAccepted   prometheus.Counter
	shareStale      prometheus.Counter
	shareInvalid    prometheus.Counter
	blocksFound     prometheus.Counter
	lastBlockHeight prometheus.Gauge
	blocksSubmitted *prometheus.CounterVec
}

// NewPromRecorder creates a Prometheus-backed Recorder and exposes a handler for metrics scraping.
// Namespace is prefixed on all metrics; if empty, "stratum" is used.
func NewPromRecorder(namespace string) (*PromRecorder, error) {
	if namespace == "" {
		namespace = "stratum"
	}
	reg := prometheus.NewRegistry()

	connOpened := prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "connections_opened_total", Help: "Total TCP connections accepted."})
	connClosed := prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "connections_closed_total", Help: "Total TCP connections closed."})
	shareAccepted := prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "shares_accepted_total", Help: "Accepted shares."})
	shareStale := prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "shares_stale_total", Help: "Stale shares."})
	shareInvalid := prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "shares_invalid_total", Help: "Invalid shares."})
	blocksFound := prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "blocks_found_total", Help: "Blocks found (candidate)."})
	lastBlockHeight := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: namespace, Name: "last_block_height", Help: "Height of the last found block."})
	blocksSubmitted := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: "block_submissions_total", Help: "Block submissions by result."}, []string{"status"})

	collectors := []prometheus.Collector{connOpened, connClosed, shareAccepted, shareStale, shareInvalid, blocksFound, lastBlockHeight, blocksSubmitted}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	return &PromRecorder{
		registry:        reg,
		handler:         promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		connOpened:      connOpened,
		connClosed:      connClosed,
		shareAccepted:   shareAccepted,
		shareStale:      shareStale,
		shareInvalid:    shareInvalid,
		blocksFound:     blocksFound,
		lastBlockHeight: lastBlockHeight,
		blocksSubmitted: blocksSubmitted,
	}, nil
}

// Handler exposes the HTTP handler for scraping.
func (p *PromRecorder) Handler() http.Handler {
	return p.handler
}

func (p *PromRecorder) ConnOpened()    { p.connOpened.Inc() }
func (p *PromRecorder) ConnClosed()    { p.connClosed.Inc() }
func (p *PromRecorder) ShareAccepted() { p.shareAccepted.Inc() }
func (p *PromRecorder) ShareStale()    { p.shareStale.Inc() }
func (p *PromRecorder) ShareInvalid()  { p.shareInvalid.Inc() }
func (p *PromRecorder) BlockFound(height int64, _ string) {
	p.blocksFound.Inc()
	p.lastBlockHeight.Set(float64(height))
}
func (p *PromRecorder) BlockSubmitted(success bool) {
	status := "failure"
	if success {
		status = "success"
	}
	p.blocksSubmitted.WithLabelValues(status).Inc()
}
