package adapter

import (
	"context"
	"time"

	"github.com/calvinpuram/latencybisect/pkg/trace"
)

type Source interface {
	Fetch(ctx context.Context, service string, start, end time.Time, limit int) ([]trace.Trace, error)
}
