package metrics

// Recorder defines the metrics hooks for the pool. The default implementation is a no-op
// to avoid forcing a backend choice at this stage.
type Recorder interface {
	ConnOpened()
	ConnClosed()
	ShareAccepted()
	ShareStale()
	ShareInvalid()
	BlockFound(height int64, jobID string)
	BlockSubmitted(success bool)
}

// NoopRecorder implements Recorder without emitting metrics.
type NoopRecorder struct{}

func (NoopRecorder) ConnOpened()                           {}
func (NoopRecorder) ConnClosed()                           {}
func (NoopRecorder) ShareAccepted()                        {}
func (NoopRecorder) ShareStale()                           {}
func (NoopRecorder) ShareInvalid()                         {}
func (NoopRecorder) BlockFound(height int64, jobID string) {}
func (NoopRecorder) BlockSubmitted(success bool)           {}

// Default is the process-wide metrics sink; replace with a real implementation when ready.
var Default Recorder = NoopRecorder{}
