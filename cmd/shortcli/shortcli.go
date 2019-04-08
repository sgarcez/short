package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"google.golang.org/grpc"

	"github.com/go-kit/kit/log"

	"github.com/sgarcez/short/pkg/shortservice"
	"github.com/sgarcez/short/pkg/shorttransport"
)

func main() {
	fs := flag.NewFlagSet("shortcli", flag.ExitOnError)
	var (
		httpAddr = fs.String("http-addr", "", "HTTP address of shortsvc")
		grpcAddr = fs.String("grpc-addr", "", "gRPC address of shortsvc")
		method   = fs.String("method", "create", "create, lookup")
	)
	fs.Usage = usageFor(fs, os.Args[0]+" [flags] <arg>")
	fs.Parse(os.Args[1:])
	if len(fs.Args()) != 1 {
		fs.Usage()
		os.Exit(1)
	}

	var (
		svc shortservice.Service
		err error
	)
	if *httpAddr != "" {
		svc, err = shorttransport.NewHTTPClient(*httpAddr, log.NewNopLogger())
	} else if *grpcAddr != "" {
		conn, err := grpc.Dial(*grpcAddr, grpc.WithInsecure(), grpc.WithTimeout(time.Second))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v", err)
			os.Exit(1)
		}
		defer conn.Close()
		svc = shorttransport.NewGRPCClient(conn, log.NewNopLogger())
	} else {
		fmt.Fprintf(os.Stderr, "error: no remote address specified\n")
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	switch *method {
	case "create":
		value := fs.Args()[0]
		k, err := svc.Create(context.Background(), value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "%s\n", k)

	case "lookup":
		k := fs.Args()[0]
		v, err := svc.Lookup(context.Background(), k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "%s\n", v)

	default:
		fmt.Fprintf(os.Stderr, "error: invalid method %q\n", *method)
		os.Exit(1)
	}
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
