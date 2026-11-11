package detect

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestProbeSOCKS5(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		request := make([]byte, 3)
		_, _ = io.ReadFull(conn, request)
		_, _ = conn.Write([]byte{0x05, 0x00})
	}()

	ok, auth := probeSOCKS5(context.Background(), listener.Addr().String(), time.Second)
	if !ok || auth {
		t.Fatalf("SOCKS5 probe: auth=%v ok=%v", auth, ok)
	}
}

func TestProbeHTTPConnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil || line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
	}()

	if !probeHTTPConnect(context.Background(), listener.Addr().String(), time.Second) {
		t.Fatal("HTTP CONNECT probe failed")
	}
}

func TestCandidateString(t *testing.T) {
	got := (Candidate{
		Software:  "Clash",
		Address:   "127.0.0.1:7890",
		Protocols: []string{"http", "socks5"},
	}).String()
	if !strings.Contains(got, "Clash") || !strings.Contains(got, "http, socks5") {
		t.Fatalf("candidate string = %q", got)
	}
}
