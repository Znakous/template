package health

import "context"

type readinessChecker interface {
	CheckReady(ctx context.Context) error
}