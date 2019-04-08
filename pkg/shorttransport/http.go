package shorttransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/gorilla/mux"
	"github.com/sony/gobreaker"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/ratelimit"
	httptransport "github.com/go-kit/kit/transport/http"

	"github.com/sgarcez/short/pkg/shortendpoint"
	"github.com/sgarcez/short/pkg/shortservice"
)

// NewHTTPHandler returns an HTTP handler that makes a set of endpoints
// available on predefined paths.
func NewHTTPHandler(endpoints shortendpoint.Set, logger log.Logger) http.Handler {

	options := []httptransport.ServerOption{
		httptransport.ServerErrorEncoder(errorEncoder),
		httptransport.ServerErrorLogger(logger),
	}

	// m := http.NewServeMux()
	r := mux.NewRouter()
	r.Methods("POST").Path("/api").Handler(httptransport.NewServer(
		endpoints.CreateEndpoint,
		decodeHTTPCreateRequest,
		encodeHTTPGenericResponse,
		options...,
	))
	r.Methods("GET").Path("/api/{key}").Handler(httptransport.NewServer(
		endpoints.LookupEndpoint,
		decodeHTTPLookupRequest,
		encodeHTTPGenericResponse,
		options...,
	))
	return r
}

// NewHTTPClient returns a Service backed by an HTTP server living at the
// remote instance. We expect instance to come from a service discovery system,
// so likely of the form "host:port". We bake-in certain middlewares,
// implementing the client library pattern.
func NewHTTPClient(instance string, logger log.Logger) (shortservice.Service, error) {
	// Quickly sanitize the instance string.
	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	u, err := url.Parse(instance)
	if err != nil {
		return nil, err
	}

	limiter := ratelimit.NewErroringLimiter(rate.NewLimiter(50, 100))
	breaker := circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Timeout: 5 * time.Second,
	}))

	// Each individual endpoint is an http/transport.Client (which implements
	// endpoint.Endpoint) that gets wrapped with various middlewares. If you
	// made your own client library, you'd do this work there, so your server
	// could rely on a consistent set of client behavior.
	var createEndpoint endpoint.Endpoint
	{
		createEndpoint = httptransport.NewClient(
			"POST",
			copyURL(u, "/api"),
			encodeHTTPCreateRequest,
			decodeHTTPCreateResponse,
		).Endpoint()
		createEndpoint = limiter(createEndpoint)
		createEndpoint = breaker(createEndpoint)
	}

	// The Lookup endpoint is the same thing, with slightly different
	// middlewares to demonstrate how to specialize per-endpoint.
	var lookupEndpoint endpoint.Endpoint
	{
		lookupEndpoint = httptransport.NewClient(
			"GET",
			copyURL(u, "/api"),
			encodeHTTPLookupRequest,
			decodeHTTPLookupResponse,
		).Endpoint()
		lookupEndpoint = limiter(lookupEndpoint)
		lookupEndpoint = breaker(lookupEndpoint)
	}

	// Returning the endpoint.Set as a service.Service relies on the
	// endpoint.Set implementing the Service methods.
	return shortendpoint.Set{
		CreateEndpoint: createEndpoint,
		LookupEndpoint: lookupEndpoint,
	}, nil
}

func copyURL(base *url.URL, path string) *url.URL {
	next := *base
	next.Path = path
	return &next
}

func errorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	w.WriteHeader(err2code(err))
	json.NewEncoder(w).Encode(errorWrapper{Error: err.Error()})
}

func err2code(err error) int {
	switch err {
	case shortservice.ErrKeyNotFound:
		return http.StatusNotFound
	case shortservice.ErrMaxSizeExceeded:
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func errorDecoder(r *http.Response) error {
	var w errorWrapper
	if err := json.NewDecoder(r.Body).Decode(&w); err != nil {
		return err
	}
	return errors.New(w.Error)
}

type errorWrapper struct {
	Error string `json:"error"`
}

// decodeHTTPCreateRequest is a transport/http.DecodeRequestFunc that decodes a
// JSON-encoded create request from the HTTP request body. Primarily useful in a
// server.
func decodeHTTPCreateRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req shortendpoint.CreateRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}

// decodeHTTPLookupRequest is a transport/http.DecodeRequestFunc that decodes a
// JSON-encoded lookup request from the HTTP request body. Primarily useful in a
// server.
func decodeHTTPLookupRequest(_ context.Context, r *http.Request) (interface{}, error) {
	vars := mux.Vars(r)
	k, _ := vars["key"]
	req := shortendpoint.LookupRequest{K: k}
	return req, nil
}

// decodeHTTPCreateResponse is a transport/http.DecodeResponseFunc that decodes a
// JSON-encoded create response from the HTTP response body. If the response has a
// non-200 status code, we will interpret that as an error and attempt to decode
// the specific error message from the response body. Primarily useful in a
// client.
func decodeHTTPCreateResponse(_ context.Context, r *http.Response) (interface{}, error) {
	if r.StatusCode != http.StatusOK {
		return nil, errors.New(r.Status)
	}
	var resp shortendpoint.CreateResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	return resp, err
}

// Primarily useful in a client.
func encodeHTTPLookupRequest(ctx context.Context, r *http.Request, request interface{}) error {
	lr, _ := request.(shortendpoint.LookupRequest)
	r.URL.Path = path.Join(r.URL.Path, lr.K)
	return nil
}

// decodeHTTPLookupResponse is a transport/http.DecodeResponseFunc that decodes
// a JSON-encoded lookup response from the HTTP response body. If the response
// has a non-200 status code, we will interpret that as an error and attempt to
// decode the specific error message from the response body. Primarily useful in
// a client.
func decodeHTTPLookupResponse(_ context.Context, r *http.Response) (interface{}, error) {
	if r.StatusCode != http.StatusOK {
		return nil, errors.New(r.Status)
	}
	var resp shortendpoint.LookupResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	return resp, err
}

// encodeHTTPCreateRequest is a transport/http.EncodeRequestFunc that
// JSON-encodes a Create request to the request body. Primarily useful in a client.
func encodeHTTPCreateRequest(_ context.Context, r *http.Request, request interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(&buf)
	return nil
}

// encodeHTTPGenericResponse is a transport/http.EncodeResponseFunc that encodes
// the response as JSON to the response writer. Primarily useful in a server.
func encodeHTTPGenericResponse(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	if f, ok := response.(endpoint.Failer); ok && f.Failed() != nil {
		errorEncoder(ctx, f.Failed(), w)
		return nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
