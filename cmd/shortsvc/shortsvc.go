package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/oklog/run"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/prometheus"
	kitgrpc "github.com/go-kit/kit/transport/grpc"

	shortpb "github.com/sgarcez/short/pb"
	"github.com/sgarcez/short/pkg/shortendpoint"
	"github.com/sgarcez/short/pkg/shortservice"
	"github.com/sgarcez/short/pkg/shorttransport"
)

func main() {
	fs := flag.NewFlagSet("shortsvc", flag.ExitOnError)
	var (
		debugAddr = fs.String("debug.addr", ":8080", "Debug and metrics listen address")
		httpAddr  = fs.String("http-addr", ":8081", "HTTP listen address")
		grpcAddr  = fs.String("grpc-addr", ":8082", "gRPC listen address")
		store     = fs.String("store", "inmem", "Storage backen type")
	)
	fs.Usage = usageFor(fs, os.Args[0]+" [flags]")
	fs.Parse(os.Args[1:])

	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = log.With(logger, "ts", log.DefaultTimestampUTC)
		logger = log.With(logger, "caller", log.DefaultCaller)
	}

	var inserts, lookups metrics.Counter
	{
		// Business-level metrics.
		// TODO: Include count of key collisions here.
		inserts = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: "default",
			Subsystem: "shortsvc",
			Name:      "inserts",
			Help:      "Total count of inserts.",
		}, []string{"method", "success"})
		lookups = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: "default",
			Subsystem: "shortsvc",
			Name:      "lookups",
			Help:      "Total count of lookups.",
		}, []string{"method", "success"})
	}
	var duration metrics.Histogram
	{
		// Endpoint-level metrics.
		duration = prometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
			Namespace: "example",
			Subsystem: "shortsvc",
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds.",
		}, []string{"method", "success"})
	}
	http.DefaultServeMux.Handle("/metrics", promhttp.Handler())

	var service shortservice.Service
	{
		switch *store {
		case "inmem":
			logger.Log("Storage", store)
			service = shortservice.NewInMemService(logger, inserts, lookups)
		default:
			logger.Log("during", "boot", "store", *store, "err", "Unsupported storage type")
			os.Exit(1)
		}
	}

	var (
		endpoints   = shortendpoint.New(service, logger, duration)
		httpHandler = shorttransport.NewHTTPHandler(endpoints, logger)
		grpcServer  = shorttransport.NewGRPCServer(endpoints, logger)
	)

	var g run.Group
	{
		// The debug listener mounts the http.DefaultServeMux, and serves up
		// the Prometheus metrics route.
		debugListener, err := net.Listen("tcp", *debugAddr)
		if err != nil {
			logger.Log("transport", "debug/HTTP", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "debug/HTTP", "addr", *debugAddr)
			return http.Serve(debugListener, http.DefaultServeMux)
		}, func(error) {
			debugListener.Close()
		})
	}
	{
		// The HTTP listener mounts the Go kit HTTP handler.
		httpListener, err := net.Listen("tcp", *httpAddr)
		if err != nil {
			logger.Log("transport", "HTTP", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "HTTP", "addr", *httpAddr)
			return http.Serve(httpListener, httpHandler)
		}, func(error) {
			httpListener.Close()
		})
	}
	{
		// The gRPC listener mounts the Go kit gRPC server.
		grpcListener, err := net.Listen("tcp", *grpcAddr)
		if err != nil {
			logger.Log("transport", "gRPC", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "gRPC", "addr", *grpcAddr)
			baseServer := grpc.NewServer(grpc.UnaryInterceptor(kitgrpc.Interceptor))
			shortpb.RegisterShortenServer(baseServer, grpcServer)
			return baseServer.Serve(grpcListener)
		}, func(error) {
			grpcListener.Close()
		})
	}
	{
		cancelInterrupt := make(chan struct{})
		g.Add(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			select {
			case sig := <-c:
				return fmt.Errorf("received signal %s", sig)
			case <-cancelInterrupt:
				return nil
			}
		}, func(error) {
			close(cancelInterrupt)
		})
	}
	logger.Log("exit", g.Run())
}

func usageFor(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "USAGE\n")
		fmt.Fprintf(os.Stderr, "  %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "\t-%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}
