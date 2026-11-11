package upstream

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestSOCKS5DialerConnectsThroughUpstream(t *testing.T) {
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

	socksListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer socksListener.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, acceptErr := socksListener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		serveSimpleSOCKS5(t, conn)
	}()

	dialer, err := New("socks5h://" + socksListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", echoListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatal(err)
	}
	if string(response) != "ping" {
		t.Fatalf("response = %q, want ping", response)
	}
	_ = conn.Close()
	wg.Wait()
}

func TestNewRejectsUnsupportedScheme(t *testing.T) {
	if _, err := New("ftp://127.0.0.1:10808"); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
}

func serveSimpleSOCKS5(t *testing.T, conn net.Conn) {
	t.Helper()
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Error(err)
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		t.Error(err)
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		t.Error(err)
		return
	}

	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		t.Error(err)
		return
	}

	var host string
	switch request[3] {
	case 0x01:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			t.Error(err)
			return
		}
		host = net.IP(ip).String()
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			t.Error(err)
			return
		}
		name := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, name); err != nil {
			t.Error(err)
			return
		}
		host = string(name)
	default:
		t.Errorf("unexpected address type %d", request[3])
		return
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		t.Error(err)
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes))))
	targetConn, err := net.DialTimeout("tcp", target, time.Second)
	if err != nil {
		t.Error(err)
		return
	}
	defer targetConn.Close()

	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0, 0}); err != nil {
		t.Error(err)
		return
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(targetConn, conn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, targetConn)
		done <- struct{}{}
	}()
	<-done
}
