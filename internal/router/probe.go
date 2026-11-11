package router

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

// ProbeFunc reports whether a direct route is usable.
type ProbeFunc func(context.Context, string) (time.Duration, error)

// NewDirectProbe checks TCP reachability and, for HTTPS, completes a TLS
// handshake. A successful TCP connection that is reset during TLS negotiation
// is therefore treated as an unavailable direct route.
func NewDirectProbe(timeout time.Duration) ProbeFunc {
	return func(ctx context.Context, address string) (time.Duration, error) {
		start := time.Now()
		dialer := &net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return time.Since(start), err
		}
		defer conn.Close()

		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return time.Since(start), err
		}
		if port != "443" {
			return time.Since(start), nil
		}

		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         host,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // The probe checks reachability, not trust.
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return time.Since(start), err
		}
		return time.Since(start), nil
	}
}
