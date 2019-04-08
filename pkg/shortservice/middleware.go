package shortservice

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
)

// Middleware describes a service (as opposed to endpoint) middleware.
type Middleware func(Service) Service

// LoggingMiddleware takes a logger as a dependency
// and returns a ServiceMiddleware.
func LoggingMiddleware(logger log.Logger) Middleware {
	return func(next Service) Service {
		return loggingMiddleware{logger, next}
	}
}

type loggingMiddleware struct {
	logger log.Logger
	next   Service
}

func (mw loggingMiddleware) Create(ctx context.Context, v string) (k string, err error) {
	defer func() {
		mw.logger.Log("method", "Create", "v", v, "k", k, "err", err)
	}()
	return mw.next.Create(ctx, v)
}

func (mw loggingMiddleware) Lookup(ctx context.Context, k string) (v string, err error) {
	defer func() {
		mw.logger.Log("method", "Lookup", "k", k, "v", v, "err", err)
	}()
	return mw.next.Lookup(ctx, k)
}

// InstrumentingMiddleware returns a service middleware that instruments
// the number of creations and lookups over the lifetime of
// the service.
func InstrumentingMiddleware(inserts, lookups metrics.Counter) Middleware {
	return func(next Service) Service {
		return instrumentingMiddleware{
			inserts: inserts,
			lookups: lookups,
			next:    next,
		}
	}
}

type instrumentingMiddleware struct {
	inserts metrics.Counter
	lookups metrics.Counter
	next    Service
}

func (mw instrumentingMiddleware) Create(ctx context.Context, v string) (string, error) {
	v, err := mw.next.Create(ctx, v)
	if err != nil {
		mw.inserts.With("method", "Create", "success", fmt.Sprint(err == nil)).Add(1)
	}
	return v, err
}

func (mw instrumentingMiddleware) Lookup(ctx context.Context, k string) (string, error) {
	v, err := mw.next.Lookup(ctx, k)
	if err != nil {
		mw.lookups.With("method", "Lookup", "success", fmt.Sprint(err == nil)).Add(1)
	}
	return v, err
}
