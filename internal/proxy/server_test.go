package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConnectTunnel(t *testing.T) {
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoListener.Close()

	go func() {
		conn, acceptErr := echoListener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	dialer := &net.Dialer{Timeout: time.Second}
	proxyServer := httptest.NewServer(&Server{
		Dial:   dialer.DialContext,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	defer proxyServer.Close()

	proxyAddress := strings.TrimPrefix(proxyServer.URL, "http://")
	conn, err := net.DialTimeout("tcp", proxyAddress, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	_, err = fmt.Fprintf(
		conn,
		"CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n",
		echoListener.Addr(),
		echoListener.Addr(),
	)
	if err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusLine, "200") {
		t.Fatalf("CONNECT status = %q", statusLine)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}

	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "ping" {
		t.Fatalf("echo = %q, want ping", buffer)
	}
}

func TestRejectsNonLoopbackClient(t *testing.T) {
	handler := &Server{
		Dial: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("dial must not be called")
			return nil, nil
		},
	}
	request := httptest.NewRequest("CONNECT", "http://example.com:443", nil)
	request.RemoteAddr = "192.0.2.10:54321"
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != 403 {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	server := httptest.NewServer(&Server{})
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/__netferry/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if string(body) != "netferry ok\n" {
		t.Fatalf("body = %q", body)
	}
}
