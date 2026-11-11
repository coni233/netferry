package proxy

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
)

type DialFunc func(context.Context, string, string) (net.Conn, error)

type Server struct {
	Dial   DialFunc
	Logger *slog.Logger
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		http.Error(w, "netferry only accepts loopback clients", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodGet && r.URL.Host == "" && r.URL.Path == "/__netferry/health" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "netferry ok\n")
		return
	}

	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}
	http.Error(w, "netferry currently requires HTTPS or CONNECT", http.StatusNotImplemented)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	target := r.Host
	if target == "" {
		http.Error(w, "missing CONNECT target", http.StatusBadRequest)
		return
	}
	if _, _, err := net.SplitHostPort(target); err != nil {
		http.Error(w, "CONNECT target must include a port", http.StatusBadRequest)
		return
	}

	upstreamConn, err := s.Dial(r.Context(), "tcp", target)
	if err != nil {
		s.logger().Warn("connect failed", "target", target, "error", err)
		http.Error(w, "all routes failed", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "connection hijacking is not supported", http.StatusInternalServerError)
		return
	}
	clientConn, rw, err := hijacker.Hijack()
	if err != nil {
		s.logger().Warn("hijack failed", "target", target, "error", err)
		return
	}
	defer clientConn.Close()

	if _, err := rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err := rw.Flush(); err != nil {
		return
	}

	relay(clientConn, rw.Reader, upstreamConn)
}

func relay(client net.Conn, clientReader *bufio.Reader, upstream net.Conn) {
	done := make(chan struct{}, 2)

	go func() {
		_, _ = io.Copy(upstream, clientReader)
		closeWrite(upstream)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		closeWrite(client)
		done <- struct{}{}
	}()

	<-done
	_ = client.Close()
	_ = upstream.Close()
	<-done
}

func closeWrite(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.CloseWrite()
	}
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

var _ http.Handler = (*Server)(nil)
