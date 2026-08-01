package health

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// Result is a SOCKS egress probe outcome.
type Result struct {
	OK        bool
	LatencyMs int64
	ExitIP    string
	Colo      string
	Raw       string
	Error     string
}

// ProbeViaSOCKS dials probeURL through a SOCKS5 proxy and parses cdn-cgi/trace style body.
func ProbeViaSOCKS(ctx context.Context, socksHost string, socksPort int, user, pass, probeURL string, timeout time.Duration) Result {
	start := time.Now()
	addr := fmt.Sprintf("%s:%d", socksHost, socksPort)

	var auth *proxy.Auth
	if user != "" {
		auth = &proxy.Auth{User: user, Password: pass}
	}
	base := &net.Dialer{Timeout: timeout}
	dialer, err := proxy.SOCKS5("tcp", addr, auth, base)
	if err != nil {
		return Result{OK: false, Error: fmt.Sprintf("socks dialer: %v", err)}
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			// proxy.Dialer is not context-aware; honor timeout via base dialer.
			return dialer.Dial(network, address)
		},
		DisableKeepAlives: true,
	}

	client := &http.Client{Transport: transport, Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return Result{OK: false, Error: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{OK: false, Error: err.Error(), LatencyMs: time.Since(start).Milliseconds()}
	}
	defer resp.Body.Close()
	latency := time.Since(start).Milliseconds()

	sc := bufio.NewScanner(resp.Body)
	var lines []string
	fields := map[string]string{}
	for sc.Scan() {
		line := sc.Text()
		lines = append(lines, line)
		if k, v, ok := strings.Cut(line, "="); ok {
			fields[k] = v
		}
		if len(lines) > 40 {
			break
		}
	}
	raw := strings.Join(lines, "\n")
	if resp.StatusCode >= 400 {
		return Result{OK: false, LatencyMs: latency, Raw: raw, Error: fmt.Sprintf("http %d", resp.StatusCode)}
	}
	return Result{
		OK:        true,
		LatencyMs: latency,
		ExitIP:    fields["ip"],
		Colo:      fields["colo"],
		Raw:       raw,
	}
}

// ProbeMock simulates health without network (local unit/integration tests).
func ProbeMock(exitIP string) Result {
	if exitIP == "" {
		exitIP = "203.0.113.10"
	}
	return Result{OK: true, LatencyMs: 1, ExitIP: exitIP, Colo: "MOCK", Raw: "ip=" + exitIP + "\ncolo=MOCK\n"}
}
