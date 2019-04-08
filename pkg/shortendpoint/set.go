package shortendpoint

import (
	"context"

	"golang.org/x/time/rate"

	"github.com/sony/gobreaker"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/ratelimit"

	"github.com/sgarcez/short/pkg/shortservice"
)

// Set collects all of the endpoints that compose a service. It's meant to
// be used as a helper struct, to collect all of the endpoints into a single
// parameter.
type Set struct {
	CreateEndpoint endpoint.Endpoint
	LookupEndpoint endpoint.Endpoint
}

// New returns a Set that wraps the provided server, and wires in all of the
// expected endpoint middlewares via the various parameters.
func New(svc shortservice.Service, logger log.Logger, duration metrics.Histogram) Set {
	var createEndpoint endpoint.Endpoint
	{
		createEndpoint = MakeCreateEndpoint(svc)
		createEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(50, 1))(createEndpoint)
		createEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(createEndpoint)
		createEndpoint = LoggingMiddleware(log.With(logger, "method", "Create"))(createEndpoint)
		createEndpoint = InstrumentingMiddleware(duration.With("method", "Create"))(createEndpoint)
	}
	var lookupEndpoint endpoint.Endpoint
	{
		lookupEndpoint = MakeLookupEndpoint(svc)
		lookupEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(100, 500))(lookupEndpoint)
		lookupEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(lookupEndpoint)
		lookupEndpoint = LoggingMiddleware(log.With(logger, "method", "Lookup"))(lookupEndpoint)
		lookupEndpoint = InstrumentingMiddleware(duration.With("method", "Lookup"))(lookupEndpoint)
	}
	return Set{
		CreateEndpoint: createEndpoint,
		LookupEndpoint: lookupEndpoint,
	}
}

// Create implements the service interface, so Set may be used as a service.
// This is primarily useful in the context of a client library.
func (s Set) Create(ctx context.Context, v string) (string, error) {
	resp, err := s.CreateEndpoint(ctx, CreateRequest{V: v})
	if err != nil {
		return "", err
	}
	response := resp.(CreateResponse)
	return response.K, response.Err
}

// Lookup implements the service interface, so Set may be used as a
// service. This is primarily useful in the context of a client library.
func (s Set) Lookup(ctx context.Context, k string) (string, error) {
	resp, err := s.LookupEndpoint(ctx, LookupRequest{K: k})
	if err != nil {
		return "", err
	}
	response := resp.(LookupResponse)
	return response.V, response.Err
}

// MakeCreateEndpoint constructs a Create endpoint wrapping the service.
func MakeCreateEndpoint(s shortservice.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(CreateRequest)
		k, err := s.Create(ctx, req.V)
		return CreateResponse{K: k, Err: err}, nil
	}
}

// MakeLookupEndpoint constructs a Lookup endpoint wrapping the service.
func MakeLookupEndpoint(s shortservice.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(LookupRequest)
		v, err := s.Lookup(ctx, req.K)
		return LookupResponse{V: v, Err: err}, nil
	}
}

// compile time assertions for our response types implementing endpoint.Failer.
var (
	_ endpoint.Failer = CreateResponse{}
	_ endpoint.Failer = LookupResponse{}
)

// CreateRequest collects the request parameters for the Create method.
type CreateRequest struct {
	V string
}

// CreateResponse collects the response values for the Create method.
type CreateResponse struct {
	K   string `json:"k"`
	Err error  `json:"-"` // should be intercepted by Failed/errorEncoder
}

// Failed implements endpoint.Failer.
func (r CreateResponse) Failed() error { return r.Err }

// LookupRequest collects the request parameters for the Lookup method.
type LookupRequest struct {
	K string
}

// LookupResponse collects the response values for the Lookup method.
type LookupResponse struct {
	V   string `json:"v"`
	Err error  `json:"-"`
}

// Failed implements endpoint.Failer.
func (r LookupResponse) Failed() error { return r.Err }
