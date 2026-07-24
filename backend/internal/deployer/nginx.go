package deployer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	routeConfirmationTimeout  = 2 * time.Second
	routeConfirmationInterval = 100 * time.Millisecond
)

type trafficRoute struct {
	Known bool
	Port  int
}

type trafficEndpoint struct {
	Port       int
	Slot       string
	AllowEmpty bool
}

func (m *Manager) switchTraffic(ctx context.Context, target, previous trafficEndpoint) (trafficRoute, error) {
	oldData, readErr := os.ReadFile(m.cfg.NginxUpstreamPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return trafficRoute{}, readErr
	}
	if err := m.writeUpstream(target.Port); err != nil {
		return trafficRoute{}, err
	}
	if err := m.runConfigured(ctx, m.cfg.NginxTestCommand); err != nil {
		restoreErr := restoreFile(m.cfg.NginxUpstreamPath, oldData)
		route, probeErr := m.observeTrafficRoute(ctx, target, previous)
		if restoreErr != nil || probeErr != nil {
			return route, fmt.Errorf("nginx configuration test failed: %w; restore file error: %v; route probe error: %v", err, restoreErr, probeErr)
		}
		return route, fmt.Errorf("nginx configuration test failed: %w", err)
	}
	reloadErr := m.runConfigured(ctx, m.cfg.NginxReloadCommand)
	routeAfterReload, probeErr := m.confirmTrafficRoute(ctx, target, previous, target.Port)
	if reloadErr == nil && probeErr == nil && routeAfterReload.Known && routeAfterReload.Port == target.Port {
		return routeAfterReload, nil
	}

	restoreFileErr := restoreFile(m.cfg.NginxUpstreamPath, oldData)
	var restoreTestErr error
	var restoreReloadErr error
	if restoreFileErr == nil {
		restoreTestErr = m.runConfigured(ctx, m.cfg.NginxTestCommand)
		if restoreTestErr == nil {
			restoreReloadErr = m.runConfigured(ctx, m.cfg.NginxReloadCommand)
		}
	}
	finalRoute, finalProbeErr := m.confirmTrafficRoute(ctx, target, previous, previous.Port)
	return finalRoute, fmt.Errorf(
		"nginx target reload was not confirmed (reload: %v, probe: %v, observed: %+v); previous configuration restoration (file: %v, test: %v, reload: %v, probe: %v)",
		reloadErr, probeErr, routeAfterReload, restoreFileErr, restoreTestErr, restoreReloadErr, finalProbeErr,
	)
}

func (m *Manager) observeTrafficRoute(ctx context.Context, target, previous trafficEndpoint) (trafficRoute, error) {
	health, err := m.fetchApplicationHealthWithClient(ctx, m.freshProbeClient(), m.cfg.NginxProbeURL, m.cfg.NginxProbeHost)
	if err != nil {
		return trafficRoute{}, err
	}
	actualSlot := strings.TrimSpace(health.DeploymentRuntime.Slot)
	if (target.Slot != "" && actualSlot == target.Slot) || (target.AllowEmpty && actualSlot == "") {
		return trafficRoute{Known: true, Port: target.Port}, nil
	}
	if (previous.Slot != "" && actualSlot == previous.Slot) || (previous.AllowEmpty && actualSlot == "") {
		return trafficRoute{Known: true, Port: previous.Port}, nil
	}
	return trafficRoute{}, fmt.Errorf("routed deployment runtime slot %q matches neither target %q nor previous %q", actualSlot, target.Slot, previous.Slot)
}

func (m *Manager) confirmTrafficRoute(ctx context.Context, target, previous trafficEndpoint, expectedPort int) (trafficRoute, error) {
	confirmationCtx, cancel := context.WithTimeout(ctx, routeConfirmationTimeout)
	defer cancel()
	var lastRoute trafficRoute
	var lastErr error
	for {
		route, err := m.observeTrafficRoute(confirmationCtx, target, previous)
		if route.Known {
			lastRoute = route
			if route.Port == expectedPort {
				return route, nil
			}
			lastErr = fmt.Errorf("observed route port %d, expected %d", route.Port, expectedPort)
		} else if err != nil {
			lastErr = err
		}
		select {
		case <-confirmationCtx.Done():
			if lastErr == nil {
				lastErr = fmt.Errorf("route port %d was not confirmed", expectedPort)
			}
			return lastRoute, fmt.Errorf("route confirmation ended: %w; last observation: %v", confirmationCtx.Err(), lastErr)
		case <-time.After(routeConfirmationInterval):
		}
	}
}

func (m *Manager) freshProbeClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	timeout := 4 * time.Second
	if m.httpClient != nil && m.httpClient.Timeout > 0 {
		timeout = m.httpClient.Timeout
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func (m *Manager) writeUpstream(port int) error {
	content := "# Managed by sub2api-deployer. Manual edits are overwritten.\n" +
		"upstream " + m.cfg.NginxUpstreamName + " {\n" +
		"    server 127.0.0.1:" + strconv.Itoa(port) + ";\n" +
		"    keepalive 64;\n" +
		"}\n"
	return atomicWrite(m.cfg.NginxUpstreamPath, []byte(content), 0644)
}

func (m *Manager) runConfigured(ctx context.Context, command []string) error {
	if len(command) == 0 {
		return nil
	}
	_, err := m.runner.Run(ctx, nil, command[0], command[1:]...)
	return err
}

func (m *Manager) runConfiguredOutput(ctx context.Context, command []string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("configured command is empty")
	}
	return m.runner.Run(ctx, nil, command[0], command[1:]...)
}

func (m *Manager) validateManagedRoute(ctx context.Context, expectedPort int) error {
	if err := m.runConfigured(ctx, m.cfg.NginxTestCommand); err != nil {
		return fmt.Errorf("nginx configuration test: %w", err)
	}
	effectiveConfig, err := m.runConfiguredOutput(ctx, m.cfg.NginxDumpCommand)
	if err != nil {
		return fmt.Errorf("dump effective nginx configuration: %w", err)
	}
	proxyCount := proxyPassDirectiveCount([]byte(effectiveConfig), m.cfg.NginxUpstreamName)
	if proxyCount != 1 {
		return fmt.Errorf("effective nginx configuration must contain exactly one active proxy_pass for managed upstream %s, found %d", m.cfg.NginxUpstreamName, proxyCount)
	}
	port, err := readManagedUpstreamPort([]byte(effectiveConfig), m.cfg.NginxUpstreamName)
	if err != nil {
		return err
	}
	if port != expectedPort {
		return fmt.Errorf("managed nginx upstream routes to port %d, expected %d", port, expectedPort)
	}
	return nil
}

func hasProxyPassDirective(data []byte, upstreamName string) bool {
	return proxyPassDirectiveCount(data, upstreamName) > 0
}

func proxyPassDirectiveCount(data []byte, upstreamName string) int {
	clean := stripNginxComments(string(data))
	pattern := `(?m)(?:^|[;{}])\s*proxy_pass\s+http://` + regexp.QuoteMeta(upstreamName) + `(?:[;/\s]|$)`
	return len(regexp.MustCompile(pattern).FindAllStringIndex(clean, -1))
}

func readManagedUpstreamPort(data []byte, upstreamName string) (int, error) {
	clean := stripNginxComments(string(data))
	startRE := regexp.MustCompile(`(?m)(?:^|[;{}])\s*upstream\s+` + regexp.QuoteMeta(upstreamName) + `\s*\{`)
	matches := startRE.FindAllStringIndex(clean, -1)
	if len(matches) != 1 {
		return 0, fmt.Errorf("managed nginx upstream %s must be defined exactly once", upstreamName)
	}
	open := strings.LastIndex(clean[matches[0][0]:matches[0][1]], "{") + matches[0][0]
	close := matchingBrace(clean, open)
	if close < 0 {
		return 0, fmt.Errorf("managed nginx upstream %s has an unterminated block", upstreamName)
	}
	block := clean[open+1 : close]
	serverRE := regexp.MustCompile(`(?m)(?:^|;)\s*server\s+((?:127\.0\.0\.1|\[::1\]):([0-9]+))\s*;`)
	servers := serverRE.FindAllStringSubmatch(block, -1)
	allServerRE := regexp.MustCompile(`(?m)(?:^|;)\s*server\s+[^;]+;`)
	allServers := allServerRE.FindAllString(block, -1)
	if len(servers) != 1 || len(allServers) != 1 {
		return 0, errors.New("managed upstream must contain exactly one loopback server directive")
	}
	port, err := strconv.Atoi(servers[0][2])
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("managed upstream contains an invalid loopback port")
	}
	return port, nil
}

func matchingBrace(value string, open int) int {
	depth := 0
	for index := open; index < len(value); index++ {
		switch value[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func stripNginxComments(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	inSingle := false
	inDouble := false
	escaped := false
	for index := 0; index < len(value); index++ {
		char := value[index]
		if escaped {
			result.WriteByte(char)
			escaped = false
			continue
		}
		if char == '\\' && (inSingle || inDouble) {
			result.WriteByte(char)
			escaped = true
			continue
		}
		if char == '\'' && !inDouble {
			inSingle = !inSingle
			result.WriteByte(char)
			continue
		}
		if char == '"' && !inSingle {
			inDouble = !inDouble
			result.WriteByte(char)
			continue
		}
		if char == '#' && !inSingle && !inDouble {
			for index < len(value) && value[index] != '\n' {
				index++
			}
			if index < len(value) {
				result.WriteByte('\n')
			}
			continue
		}
		result.WriteByte(char)
	}
	return result.String()
}

func restoreFile(path string, data []byte) error {
	if data == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return atomicWrite(path, data, 0644)
}
