package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

type Route string

const (
	RouteDirect Route = "direct"
	RouteProxy  Route = "proxy"
)

type DialFunc func(context.Context, string, string) (net.Conn, error)

type Options struct {
	ProbeTimeout time.Duration
	CacheTTL     time.Duration
	Probe        ProbeFunc
	DirectDial   DialFunc
	ProxyDial    DialFunc
	Logger       *slog.Logger
	Now          func() time.Time
}

type cacheEntry struct {
	route     Route
	expiresAt time.Time
}

type Router struct {
	mu           sync.RWMutex
	cache        map[string]cacheEntry
	probeTimeout time.Duration
	cacheTTL     time.Duration
	probe        ProbeFunc
	directDial   DialFunc
	proxyDial    DialFunc
	logger       *slog.Logger
	now          func() time.Time
}

func New(options Options) *Router {
	if options.ProbeTimeout <= 0 {
		options.ProbeTimeout = 1500 * time.Millisecond
	}
	if options.CacheTTL <= 0 {
		options.CacheTTL = 5 * time.Minute
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.DirectDial == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		options.DirectDial = dialer.DialContext
	}
	if options.Probe == nil {
		options.Probe = NewDirectProbe(options.ProbeTimeout)
	}
	if options.ProxyDial == nil {
		options.ProxyDial = func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("proxy dialer is not configured")
		}
	}

	return &Router{
		cache:        make(map[string]cacheEntry),
		probeTimeout: options.ProbeTimeout,
		cacheTTL:     options.CacheTTL,
		probe:        options.Probe,
		directDial:   options.DirectDial,
		proxyDial:    options.ProxyDial,
		logger:       options.Logger,
		now:          options.Now,
	}
}

// DialContext chooses a route before opening the actual connection. Retries
// happen only while establishing a connection; application data is never
// replayed by the router.
func (r *Router) DialContext(ctx context.Context, network, address string) (net.Conn, Route, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, "", fmt.Errorf("parse destination %q: %w", address, err)
	}

	route, cached := r.cachedRoute(host)
	if !cached {
		probeCtx, cancel := context.WithTimeout(ctx, r.probeTimeout)
		_, probeErr := r.probe(probeCtx, address)
		cancel()
		if probeErr == nil {
			route = RouteDirect
			r.logger.Info("route selected", "host", host, "route", route, "reason", "direct probe succeeded")
		} else {
			route = RouteProxy
			r.logger.Info("route selected", "host", host, "route", route, "reason", "direct probe failed", "error", probeErr)
		}
		r.remember(host, route)
	}

	if route == RouteProxy {
		conn, proxyErr := r.proxyDial(ctx, network, address)
		if proxyErr == nil {
			return conn, RouteProxy, nil
		}
		r.logger.Warn("proxy route failed; trying direct", "host", host, "error", proxyErr)
		conn, directErr := r.directDial(ctx, network, address)
		if directErr == nil {
			r.remember(host, RouteDirect)
			return conn, RouteDirect, nil
		}
		return nil, "", fmt.Errorf("proxy route failed: %w; direct route failed: %v", proxyErr, directErr)
	}

	conn, directErr := r.directDial(ctx, network, address)
	if directErr == nil {
		return conn, RouteDirect, nil
	}
	r.logger.Warn("direct route failed; trying proxy", "host", host, "error", directErr)
	conn, proxyErr := r.proxyDial(ctx, network, address)
	if proxyErr == nil {
		r.remember(host, RouteProxy)
		return conn, RouteProxy, nil
	}
	return nil, "", fmt.Errorf("direct route failed: %w; proxy route failed: %v", directErr, proxyErr)
}

func (r *Router) cachedRoute(host string) (Route, bool) {
	key := strings.ToLower(host)
	r.mu.RLock()
	entry, ok := r.cache[key]
	r.mu.RUnlock()
	if !ok || !r.now().Before(entry.expiresAt) {
		return "", false
	}
	return entry.route, true
}

func (r *Router) remember(host string, route Route) {
	key := strings.ToLower(host)
	r.mu.Lock()
	r.cache[key] = cacheEntry{
		route:     route,
		expiresAt: r.now().Add(r.cacheTTL),
	}
	r.mu.Unlock()
}
