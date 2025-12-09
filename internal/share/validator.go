package share

import "context"

// Validator checks submitted shares against the current job and difficulty.
type Validator interface {
	Validate(ctx context.Context, submission []byte) error
}
