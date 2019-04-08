package shorttransport

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"

	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/ratelimit"
	grpctransport "github.com/go-kit/kit/transport/grpc"

	"github.com/sgarcez/short/pb"
	"github.com/sgarcez/short/pkg/shortendpoint"
	"github.com/sgarcez/short/pkg/shortservice"
)

type grpcServer struct {
	create grpctransport.Handler
	lookup grpctransport.Handler
}

// NewGRPCServer makes a set of endpoints available as a gRPC ShortenServer.
func NewGRPCServer(endpoints shortendpoint.Set, logger log.Logger) pb.ShortenServer {

	options := []grpctransport.ServerOption{
		grpctransport.ServerErrorLogger(logger),
	}

	return &grpcServer{
		create: grpctransport.NewServer(
			endpoints.CreateEndpoint,
			decodeGRPCCreateRequest,
			encodeGRPCCreateResponse,
			options...,
		),
		lookup: grpctransport.NewServer(
			endpoints.LookupEndpoint,
			decodeGRPCLookupRequest,
			encodeGRPCLookupResponse,
			options...,
		),
	}
}

func (s *grpcServer) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateReply, error) {
	_, rep, err := s.create.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return rep.(*pb.CreateReply), nil
}

func (s *grpcServer) Lookup(ctx context.Context, req *pb.LookupRequest) (*pb.LookupReply, error) {
	_, rep, err := s.lookup.ServeGRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	return rep.(*pb.LookupReply), nil
}

// NewGRPCClient returns a ShortService backed by a gRPC server at the other end
// of the conn. The caller is responsible for constructing the conn, and
// eventually closing the underlying transport. We bake-in certain middlewares,
// implementing the client library pattern.
func NewGRPCClient(conn *grpc.ClientConn, logger log.Logger) shortservice.Service {

	limiter := ratelimit.NewErroringLimiter(rate.NewLimiter(50, 100))

	// Each individual endpoint is an grpc/transport.Client (which implements
	// endpoint.Endpoint) that gets wrapped with various middlewares.
	var createEndpoint endpoint.Endpoint
	{
		createEndpoint = grpctransport.NewClient(
			conn,
			"pb.Shorten",
			"Create",
			encodeGRPCCreateRequest,
			decodeGRPCCreateResponse,
			pb.CreateReply{},
		).Endpoint()
		createEndpoint = limiter(createEndpoint)
		createEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Create",
			Timeout: 30 * time.Second,
		}))(createEndpoint)
	}

	var lookupEndpoint endpoint.Endpoint
	{
		lookupEndpoint = grpctransport.NewClient(
			conn,
			"pb.Shorten",
			"Lookup",
			encodeGRPCLookupRequest,
			decodeGRPCLookupResponse,
			pb.LookupReply{},
		).Endpoint()
		lookupEndpoint = limiter(lookupEndpoint)
		lookupEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Lookup",
			Timeout: 5 * time.Second,
		}))(lookupEndpoint)
	}

	return shortendpoint.Set{
		CreateEndpoint: createEndpoint,
		LookupEndpoint: lookupEndpoint,
	}
}

// decodeGRPCCreateRequest is a transport/grpc.DecodeRequestFunc that converts a
// gRPC Create request to a user-domain Create request. Primarily useful in a server.
func decodeGRPCCreateRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.CreateRequest)
	return shortendpoint.CreateRequest{V: req.V}, nil
}

// decodeGRPCLookupRequest is a transport/grpc.DecodeRequestFunc that converts a
// gRPC lookup request to a user-domain lookup request. Primarily useful in a
// server.
func decodeGRPCLookupRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.LookupRequest)
	return shortendpoint.LookupRequest{K: req.K}, nil
}

// decodeGRPCCreateResponse is a transport/grpc.DecodeResponseFunc that converts a
// gRPC Create reply to a user-domain Create response. Primarily useful in a client.
func decodeGRPCCreateResponse(_ context.Context, grpcReply interface{}) (interface{}, error) {
	reply := grpcReply.(*pb.CreateReply)
	return shortendpoint.CreateResponse{K: reply.K, Err: str2err(reply.Err)}, nil
}

// decodeGRPCLookupResponse is a transport/grpc.DecodeResponseFunc that converts
// a gRPC lookup reply to a user-domain lookup response. Primarily useful in a
// client.
func decodeGRPCLookupResponse(_ context.Context, grpcReply interface{}) (interface{}, error) {
	reply := grpcReply.(*pb.LookupReply)
	return shortendpoint.LookupResponse{V: reply.V, Err: str2err(reply.Err)}, nil
}

// encodeGRPCCreateResponse is a transport/grpc.EncodeResponseFunc that converts a
// user-domain Create response to a gRPC Create reply. Primarily useful in a server.
func encodeGRPCCreateResponse(_ context.Context, response interface{}) (interface{}, error) {
	resp := response.(shortendpoint.CreateResponse)
	return &pb.CreateReply{K: resp.K, Err: err2str(resp.Err)}, nil
}

// encodeGRPCLookupResponse is a transport/grpc.EncodeResponseFunc that converts
// a user-domain lookup response to a gRPC lookup reply. Primarily useful in a
// server.
func encodeGRPCLookupResponse(_ context.Context, response interface{}) (interface{}, error) {
	resp := response.(shortendpoint.LookupResponse)
	return &pb.LookupReply{V: resp.V, Err: err2str(resp.Err)}, nil
}

// encodeGRPCCreateRequest is a transport/grpc.EncodeRequestFunc that converts a
// user-domain Create request to a gRPC Create request. Primarily useful in a client.
func encodeGRPCCreateRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(shortendpoint.CreateRequest)
	return &pb.CreateRequest{V: req.V}, nil
}

// encodeGRPCLookupRequest is a transport/grpc.EncodeRequestFunc that converts a
// user-domain lookup request to a gRPC lookup request. Primarily useful in a
// client.
func encodeGRPCLookupRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(shortendpoint.LookupRequest)
	return &pb.LookupRequest{K: req.K}, nil
}

func str2err(s string) error {
	if s == "" {
		return nil
	}
	return errors.New(s)
}

func err2str(err error) string {
	switch err {
	case shortservice.ErrKeyNotFound, shortservice.ErrMaxSizeExceeded:
		return err.Error()
	}
	return "Internal server error"
}
