package detect

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/coni233/netferry/internal/config"
)

type Target struct {
	Software string
	Name     string
	Address  string
	Priority int
}

type Candidate struct {
	Software   string   `json:"software"`
	Name       string   `json:"name"`
	Address    string   `json:"address"`
	Protocols  []string `json:"protocols"`
	Upstream   string   `json:"upstream,omitempty"`
	AuthNeeded bool     `json:"auth_required,omitempty"`
}

func CommonTargets() []Target {
	return []Target{
		{Software: "v2rayN", Name: "mixed proxy", Address: "127.0.0.1:10808", Priority: 10},
		{Software: "Clash / Clash Verge / Mihomo", Name: "mixed proxy", Address: "127.0.0.1:7890", Priority: 20},
		{Software: "Clash Verge Rev / Kiri Verge", Name: "mixed proxy", Address: "127.0.0.1:7897", Priority: 25},
		{Software: "Shadowsocks", Name: "SOCKS5 proxy", Address: "127.0.0.1:1080", Priority: 30},
		{Software: "sing-box / NekoBox", Name: "mixed proxy", Address: "127.0.0.1:2080", Priority: 40},
		{Software: "v2rayN / V2Ray", Name: "HTTP proxy", Address: "127.0.0.1:10809", Priority: 50},
		{Software: "Clash / Mihomo", Name: "SOCKS5 proxy", Address: "127.0.0.1:7891", Priority: 60},
	}
}

func Detect(ctx context.Context, timeout time.Duration) []Candidate {
	return DetectExcept(ctx, timeout)
}

// DetectExcept probes discovery sources while ignoring endpoints that NetFerry
// itself may listen on, which prevents an upstream loop.
func DetectExcept(ctx context.Context, timeout time.Duration, skip ...string) []Candidate {
	targets := append(systemTargets(), CommonTargets()...)
	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].Priority < targets[j].Priority
	})

	skipped := map[string]bool{
		normalizeProxyAddress(config.DefaultListen): true,
	}
	for _, address := range skip {
		skipped[normalizeProxyAddress(address)] = true
	}
	seen := make(map[string]bool, len(targets))
	var candidates []Candidate
	for _, target := range targets {
		address := normalizeProxyAddress(target.Address)
		if address == "" || skipped[address] || seen[address] {
			continue
		}
		seen[address] = true

		protocols, authNeeded, ok := Probe(ctx, target.Address, timeout)
		if !ok {
			continue
		}
		candidate := Candidate{
			Software:   target.Software,
			Name:       target.Name,
			Address:    target.Address,
			Protocols:  protocols,
			AuthNeeded: authNeeded,
		}
		for _, protocol := range protocols {
			switch protocol {
			case "socks5":
				if !authNeeded {
					candidate.Upstream = "socks5h://" + target.Address
				}
			case "http":
				if candidate.Upstream == "" {
					candidate.Upstream = "http://" + target.Address
				}
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func Probe(ctx context.Context, address string, timeout time.Duration) ([]string, bool, bool) {
	socksOK, authNeeded := probeSOCKS5(ctx, address, timeout)
	httpOK := probeHTTPConnect(ctx, address, timeout)

	protocols := make([]string, 0, 2)
	if socksOK {
		protocols = append(protocols, "socks5")
	}
	if httpOK {
		protocols = append(protocols, "http")
	}
	return protocols, authNeeded, len(protocols) > 0
}

func probeSOCKS5(ctx context.Context, address string, timeout time.Duration) (bool, bool) {
	conn, err := dial(ctx, address, timeout)
	if err != nil {
		return false, false
	}
	defer conn.Close()

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return false, false
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return false, false
	}
	if reply[0] != 0x05 {
		return false, false
	}
	return true, reply[1] != 0x00
}

func probeHTTPConnect(ctx context.Context, address string, timeout time.Duration) bool {
	conn, err := dial(ctx, address, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	request := "CONNECT 127.0.0.1:1 HTTP/1.1\r\n" +
		"Host: 127.0.0.1:1\r\n" +
		"Proxy-Connection: close\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		return false
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return false
	}
	return strings.HasPrefix(line, "HTTP/")
}

func dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", address)
	if err != nil {
		return nil, err
	}
	deadline, ok := probeCtx.Deadline()
	if ok {
		_ = conn.SetDeadline(deadline)
	}
	return conn, nil
}

func (candidate Candidate) String() string {
	protocols := strings.Join(candidate.Protocols, ", ")
	if protocols == "" {
		protocols = "unknown"
	}
	return fmt.Sprintf("%s - %s (%s)", candidate.Software, candidate.Address, protocols)
}
