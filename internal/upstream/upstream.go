package upstream

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// New builds an upstream dialer from an HTTP or SOCKS5 proxy URL.
func New(raw string) (Dialer, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("upstream %q has no host", raw)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h":
		if parsed.User != nil {
			return nil, errors.New("authenticated SOCKS5 upstreams are not supported yet")
		}
		return &socks5Dialer{address: parsed.Host}, nil
	case "http":
		return &httpConnectDialer{
			address:  parsed.Host,
			userInfo: parsed.User,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported upstream scheme %q; use http, socks5, or socks5h", parsed.Scheme)
	}
}

type socks5Dialer struct {
	address string
}

func (d *socks5Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", d.address)
	if err != nil {
		return nil, fmt.Errorf("dial SOCKS5 upstream: %w", err)
	}
	clearDeadlineAfterHandshake := applyHandshakeDeadline(ctx, conn)

	if err := socks5Handshake(conn, address); err != nil {
		_ = conn.Close()
		return nil, err
	}
	clearDeadlineAfterHandshake()
	return conn, nil
}

func socks5Handshake(conn net.Conn, target string) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return fmt.Errorf("write SOCKS5 greeting: %w", err)
	}

	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("read SOCKS5 greeting: %w", err)
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		return fmt.Errorf("SOCKS5 upstream rejected authentication method: %v", reply)
	}

	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("parse SOCKS5 target: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid SOCKS5 target port %q", portText)
	}

	request := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			request = append(request, 0x01)
			request = append(request, ip4...)
		} else {
			request = append(request, 0x04)
			request = append(request, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return errors.New("SOCKS5 target hostname is too long")
		}
		request = append(request, 0x03, byte(len(host)))
		request = append(request, host...)
	}
	request = append(request, byte(port>>8), byte(port))

	if _, err := conn.Write(request); err != nil {
		return fmt.Errorf("write SOCKS5 connect request: %w", err)
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read SOCKS5 connect reply: %w", err)
	}
	if header[0] != 0x05 {
		return fmt.Errorf("unexpected SOCKS5 reply version %d", header[0])
	}
	if header[1] != 0x00 {
		return fmt.Errorf("SOCKS5 connect failed: %s", socks5Error(header[1]))
	}

	var addressLength int
	switch header[3] {
	case 0x01:
		addressLength = 4
	case 0x04:
		addressLength = 16
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return fmt.Errorf("read SOCKS5 bound hostname length: %w", err)
		}
		addressLength = int(length[0])
	default:
		return fmt.Errorf("unsupported SOCKS5 address type %d", header[3])
	}

	if _, err := io.CopyN(io.Discard, conn, int64(addressLength+2)); err != nil {
		return fmt.Errorf("read SOCKS5 bound address: %w", err)
	}
	return nil
}

func socks5Error(code byte) string {
	switch code {
	case 0x01:
		return "general server failure"
	case 0x02:
		return "connection not allowed"
	case 0x03:
		return "network unreachable"
	case 0x04:
		return "host unreachable"
	case 0x05:
		return "connection refused"
	case 0x06:
		return "TTL expired"
	case 0x07:
		return "command not supported"
	case 0x08:
		return "address type not supported"
	default:
		return "unknown error"
	}
}

type httpConnectDialer struct {
	address  string
	userInfo *url.Userinfo
}

func (d *httpConnectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", d.address)
	if err != nil {
		return nil, fmt.Errorf("dial HTTP upstream: %w", err)
	}

	clearDeadlineAfterHandshake := applyHandshakeDeadline(ctx, conn)
	var request strings.Builder
	fmt.Fprintf(&request, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Connection: Keep-Alive\r\n", address, address)
	if d.userInfo != nil {
		password, _ := d.userInfo.Password()
		token := base64.StdEncoding.EncodeToString([]byte(d.userInfo.Username() + ":" + password))
		fmt.Fprintf(&request, "Proxy-Authorization: Basic %s\r\n", token)
	}
	request.WriteString("\r\n")
	if _, err := io.WriteString(conn, request.String()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write HTTP CONNECT request: %w", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read HTTP CONNECT status: %w", err)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("read HTTP CONNECT headers: %w", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	fields := strings.Fields(statusLine)
	if len(fields) < 2 || fields[1] != "200" {
		_ = conn.Close()
		return nil, fmt.Errorf("HTTP CONNECT upstream rejected request: %s", strings.TrimSpace(statusLine))
	}

	clearDeadlineAfterHandshake()
	return conn, nil
}

func applyHandshakeDeadline(ctx context.Context, conn net.Conn) func() {
	deadline := time.Now().Add(10 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = conn.SetDeadline(deadline)
	return func() {
		_ = conn.SetDeadline(time.Time{})
	}
}
