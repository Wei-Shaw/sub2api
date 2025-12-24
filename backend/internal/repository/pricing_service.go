package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type pricingRemoteClient struct {
	httpClient *http.Client
REDACTED

func NewPricingRemoteClient() service.PricingRemoteClient {
	return &pricingRemoteClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
	REDACTED,
REDACTED
REDACTED

func (c *pricingRemoteClient) FetchPricingJSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
REDACTED

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
REDACTED

	return io.ReadAll(resp.Body)
REDACTED

func (c *pricingRemoteClient) FetchHashText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
REDACTED

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
REDACTED

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
REDACTED

	// 哈希文件格式：hash  filename 或者纯 hash
	hash := strings.TrimSpace(string(body))
	parts := strings.Fields(hash)
	if len(parts) > 0 {
		return parts[0], nil
REDACTED
	return hash, nil
REDACTED
