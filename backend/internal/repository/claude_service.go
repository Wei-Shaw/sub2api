package repository

import (
	"net/http"
	"net/url"
	"time"

	"sub2api/internal/config"
	"sub2api/internal/service"
)

type claudeUpstreamService struct {
	defaultClient *http.Client
	cfg           *config.Config
REDACTED

func NewClaudeUpstream(cfg *config.Config) service.ClaudeUpstream {
	responseHeaderTimeout := time.Duration(cfg.Gateway.ResponseHeaderTimeout) * time.Second
	if responseHeaderTimeout == 0 {
		responseHeaderTimeout = 300 * time.Second
REDACTED

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
REDACTED

	return &claudeUpstreamService{
		defaultClient: &http.Client{Transport: transportREDACTED,
		cfg:           cfg,
REDACTED
REDACTED

func (s *claudeUpstreamService) Do(req *http.Request, proxyURL string) (*http.Response, error) {
	if proxyURL == "" {
		return s.defaultClient.Do(req)
REDACTED
	client := s.createProxyClient(proxyURL)
	return client.Do(req)
REDACTED

func (s *claudeUpstreamService) createProxyClient(proxyURL string) *http.Client {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return s.defaultClient
REDACTED

	responseHeaderTimeout := time.Duration(s.cfg.Gateway.ResponseHeaderTimeout) * time.Second
	if responseHeaderTimeout == 0 {
		responseHeaderTimeout = 300 * time.Second
REDACTED

	transport := &http.Transport{
		Proxy:                 http.ProxyURL(parsedURL),
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
REDACTED

	return &http.Client{Transport: transportREDACTED
REDACTED
