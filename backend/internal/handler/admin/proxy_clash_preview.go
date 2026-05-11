package admin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

const clashSubscriptionMaxBytes = 2 << 20

type clashPreviewRequest struct {
	URL     string `json:"url"`
	Content string `json:"content"`
}

type clashProxyDocument struct {
	Proxies []clashProxyEntry `yaml:"proxies"`
}

type clashProxyEntry struct {
	Name     any `yaml:"name"`
	Type     any `yaml:"type"`
	Server   any `yaml:"server"`
	Port     any `yaml:"port"`
	Username any `yaml:"username"`
	Password any `yaml:"password"`
	TLS      any `yaml:"tls"`
	Enabled  any `yaml:"enabled"`
}

type ClashProxyPreviewRow struct {
	Index     int       `json:"index"`
	Name      string    `json:"name"`
	Duplicate bool      `json:"duplicate"`
	Valid     bool      `json:"valid"`
	Errors    []string  `json:"errors"`
	Proxy     DataProxy `json:"proxy"`
}

type ClashProxyPreviewSummary struct {
	Total      int `json:"total"`
	Valid      int `json:"valid"`
	Invalid    int `json:"invalid"`
	Duplicates int `json:"duplicates"`
}

type ClashBatchPayload struct {
	Proxies []ClashBatchProxy `json:"proxies"`
}

type ClashBatchProxy struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ClashProxyPreviewResponse struct {
	Source       string                   `json:"source"`
	Rows         []ClashProxyPreviewRow   `json:"rows"`
	Summary      ClashProxyPreviewSummary `json:"summary"`
	DataPayload  DataPayload              `json:"data_payload"`
	BatchPayload ClashBatchPayload        `json:"batch_payload"`
}

// PreviewClashImport parses pasted Clash YAML or fetches a Clash subscription URL.
// It only returns import-ready payloads; writes still go through existing proxy APIs.
func (h *ProxyHandler) PreviewClashImport(c *gin.Context) {
	var req clashPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	content := strings.TrimSpace(req.Content)
	source := "content"
	if content == "" {
		rawURL := strings.TrimSpace(req.URL)
		if rawURL == "" {
			response.BadRequest(c, "subscription url or yaml content is required")
			return
		}

		fetched, err := fetchClashSubscription(c.Request.Context(), rawURL)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		content = fetched
		source = "url"
	} else if len(content) > clashSubscriptionMaxBytes {
		response.BadRequest(c, "subscription content is too large")
		return
	}

	preview, err := buildClashProxyPreview(content, source)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, preview)
}

func fetchClashSubscription(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("subscription url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("subscription url must use http or https")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("subscription url is invalid")
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "clash.meta/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch subscription failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("fetch subscription failed: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, clashSubscriptionMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read subscription failed: %w", err)
	}
	if len(body) > clashSubscriptionMaxBytes {
		return "", fmt.Errorf("subscription content is too large")
	}
	return string(body), nil
}

func buildClashProxyPreview(content, source string) (ClashProxyPreviewResponse, error) {
	var doc clashProxyDocument
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return ClashProxyPreviewResponse{}, fmt.Errorf("parse Clash YAML failed: %w", err)
	}
	if len(doc.Proxies) == 0 {
		return ClashProxyPreviewResponse{}, fmt.Errorf("YAML does not contain proxies")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	out := ClashProxyPreviewResponse{
		Source: source,
		Rows:   make([]ClashProxyPreviewRow, 0, len(doc.Proxies)),
		DataPayload: DataPayload{
			Type:       dataType,
			Version:    dataVersion,
			ExportedAt: now,
			Proxies:    []DataProxy{},
			Accounts:   []DataAccount{},
		},
		BatchPayload: ClashBatchPayload{Proxies: []ClashBatchProxy{}},
	}

	seen := make(map[string]int, len(doc.Proxies))
	for i := range doc.Proxies {
		row := buildClashPreviewRow(doc.Proxies[i], i+1)
		if row.Valid {
			key := row.Proxy.ProxyKey
			if firstIndex, ok := seen[key]; ok {
				row.Duplicate = true
				row.Valid = false
				row.Errors = append(row.Errors, fmt.Sprintf("duplicate proxy, first seen at row %d", firstIndex))
				out.Summary.Duplicates++
			} else {
				seen[key] = row.Index
				out.DataPayload.Proxies = append(out.DataPayload.Proxies, row.Proxy)
				out.BatchPayload.Proxies = append(out.BatchPayload.Proxies, ClashBatchProxy{
					Protocol: row.Proxy.Protocol,
					Host:     row.Proxy.Host,
					Port:     row.Proxy.Port,
					Username: row.Proxy.Username,
					Password: row.Proxy.Password,
				})
			}
		}

		if row.Valid {
			out.Summary.Valid++
		} else {
			out.Summary.Invalid++
		}
		out.Rows = append(out.Rows, row)
	}
	out.Summary.Total = len(out.Rows)
	return out, nil
}

func buildClashPreviewRow(entry clashProxyEntry, index int) ClashProxyPreviewRow {
	name := strings.TrimSpace(clashScalarString(entry.Name))
	rawType := strings.ToLower(strings.TrimSpace(clashScalarString(entry.Type)))
	host := strings.TrimSpace(clashScalarString(entry.Server))
	port, portOK := clashScalarInt(entry.Port)
	username := strings.TrimSpace(clashScalarString(entry.Username))
	password := strings.TrimSpace(clashScalarString(entry.Password))
	tls := clashScalarBool(entry.TLS)

	status := service.StatusActive
	if enabled, ok := clashOptionalBool(entry.Enabled); ok && !enabled {
		status = "inactive"
	}

	protocol, protocolOK := mapClashProxyProtocol(rawType, tls)
	proxy := DataProxy{
		Name:     defaultProxyName(name),
		Protocol: protocol,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Status:   status,
	}
	proxy.ProxyKey = buildProxyKey(proxy.Protocol, proxy.Host, proxy.Port, proxy.Username, proxy.Password)

	row := ClashProxyPreviewRow{
		Index:  index,
		Name:   name,
		Proxy:  proxy,
		Errors: []string{},
	}
	if !protocolOK {
		if rawType == "" {
			row.Errors = append(row.Errors, "proxy type is required")
		} else {
			row.Errors = append(row.Errors, "unsupported proxy type: "+rawType)
		}
	}
	if host == "" {
		row.Errors = append(row.Errors, "server is required")
	}
	if !portOK || port <= 0 || port > 65535 {
		row.Errors = append(row.Errors, "port is invalid")
	}
	if len(row.Errors) == 0 {
		if err := validateDataProxy(proxy); err != nil {
			row.Errors = append(row.Errors, err.Error())
		}
	}
	row.Valid = len(row.Errors) == 0
	return row
}

func mapClashProxyProtocol(rawType string, tls bool) (string, bool) {
	switch rawType {
	case "http":
		if tls {
			return "https", true
		}
		return "http", true
	case "https":
		return "https", true
	case "socks5":
		if tls {
			return "socks5h", true
		}
		return "socks5", true
	case "socks5h":
		return "socks5h", true
	default:
		return "", false
	}
}

func clashScalarString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

func clashScalarInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		if v < 0 || v > 65535 {
			return 0, false
		}
		return int(v), true
	case uint64:
		if v > 65535 {
			return 0, false
		}
		return int(v), true
	case float64:
		if v != float64(int(v)) {
			return 0, false
		}
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	default:
		return 0, false
	}
}

func clashScalarBool(value any) bool {
	if parsed, ok := clashOptionalBool(value); ok {
		return parsed
	}
	return false
}

func clashOptionalBool(value any) (bool, bool) {
	switch v := value.(type) {
	case nil:
		return false, false
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}
