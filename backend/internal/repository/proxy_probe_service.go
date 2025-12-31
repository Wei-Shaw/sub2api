package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func NewProxyExitInfoProber() service.ProxyExitInfoProber {
	return &proxyProbeService{ipInfoURL: defaultIPInfoURLREDACTED
REDACTED

const defaultIPInfoURL = "https://ipinfo.io/json"

type proxyProbeService struct {
	ipInfoURL string
REDACTED

func (s *proxyProbeService) ProbeProxy(ctx context.Context, proxyURL string) (*service.ProxyExitInfo, int64, error) {
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:           proxyURL,
		Timeout:            15 * time.Second,
		InsecureSkipVerify: true,
		ProxyStrict:        true,
REDACTED)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create proxy client: %w", err)
REDACTED

	startTime := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", s.ipInfoURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
REDACTED

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("proxy connection failed: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	latencyMs := time.Since(startTime).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		return nil, latencyMs, fmt.Errorf("request failed with status: %d", resp.StatusCode)
REDACTED

	var ipInfo struct {
		IP      string `json:"ip"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
REDACTED

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latencyMs, fmt.Errorf("failed to read response: %w", err)
REDACTED

	if err := json.Unmarshal(body, &ipInfo); err != nil {
		return nil, latencyMs, fmt.Errorf("failed to parse response: %w", err)
REDACTED

	return &service.ProxyExitInfo{
		IP:      ipInfo.IP,
		City:    ipInfo.City,
		Region:  ipInfo.Region,
		Country: ipInfo.Country,
REDACTED, latencyMs, nil
REDACTED
