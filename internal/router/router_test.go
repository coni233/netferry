package router

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestRouterFallsBackToProxyAndCachesChoice(t *testing.T) {
	var probeCalls atomic.Int32
	var proxyCalls atomic.Int32
	var directCalls atomic.Int32

	r := New(Options{
		ProbeTimeout: time.Second,
		CacheTTL:     time.Minute,
		Probe: func(context.Context, string) (time.Duration, error) {
			probeCalls.Add(1)
			return time.Millisecond, errors.New("blocked")
		},
		DirectDial: func(context.Context, string, string) (net.Conn, error) {
			directCalls.Add(1)
			return nil, errors.New("direct should not be used")
		},
		ProxyDial: func(context.Context, string, string) (net.Conn, error) {
			proxyCalls.Add(1)
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	for range 2 {
		conn, route, err := r.DialContext(context.Background(), "tcp", "github.com:443")
		if err != nil {
			t.Fatalf("DialContext() error = %v", err)
		}
		_ = conn.Close()
		if route != RouteProxy {
			t.Fatalf("route = %q, want %q", route, RouteProxy)
		}
	}

	if got := probeCalls.Load(); got != 1 {
		t.Fatalf("probe calls = %d, want 1", got)
	}
	if got := proxyCalls.Load(); got != 2 {
		t.Fatalf("proxy calls = %d, want 2", got)
	}
	if got := directCalls.Load(); got != 0 {
		t.Fatalf("direct calls = %d, want 0", got)
	}
}

func TestRouterRetriesTheOtherRouteBeforeDataTransfer(t *testing.T) {
	var directCalls atomic.Int32

	r := New(Options{
		ProbeTimeout: time.Second,
		CacheTTL:     time.Minute,
		Probe: func(context.Context, string) (time.Duration, error) {
			return time.Millisecond, nil
		},
		DirectDial: func(context.Context, string, string) (net.Conn, error) {
			directCalls.Add(1)
			return nil, errors.New("direct dial failed")
		},
		ProxyDial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	conn, route, err := r.DialContext(context.Background(), "tcp", "api.github.com:443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	_ = conn.Close()
	if route != RouteProxy {
		t.Fatalf("route = %q, want %q", route, RouteProxy)
	}
	if got := directCalls.Load(); got != 1 {
		t.Fatalf("direct calls = %d, want 1", got)
	}
}
