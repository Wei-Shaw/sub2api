package xai

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const billingRequestTimeout = 15 * time.Second

// BillingRESTEndpoint is the Grok CLI proxy billing API.
var BillingRESTEndpoint = "https://cli-chat-proxy.grok.com/v1/billing"

// BillingWebGRPCEndpoint is the grok.com billing gRPC-web fallback used by CodexBar.
var BillingWebGRPCEndpoint = "https://grok.com/grok_api_v2.GrokBuildBilling/GetGrokCreditsConfig"

// BillingSummary is the normalized xAI/Grok monthly billing snapshot.
type BillingSummary struct {
	MonthlyLimitCents  *float64
	UsedCents          *float64
	OnDemandCapCents   *float64
	BillingPeriodStart *time.Time
	BillingPeriodEnd   *time.Time
	UsedPercent        *float64
	Source             string
}

// BillingHTTPError represents a non-2xx billing HTTP response.
type BillingHTTPError struct {
	StatusCode int
	Body       string
}

func (e *BillingHTTPError) Error() string {
	if e == nil {
		return ""
	}
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	if len(body) > 300 {
		body = body[:300] + "..."
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, body)
}

type billingEnvelope struct {
	Config *billingConfig `json:"config"`
}

type billingConfig struct {
	MonthlyLimit            any    `json:"monthlyLimit"`
	MonthlyLimitSnake       any    `json:"monthly_limit"`
	Used                    any    `json:"used"`
	OnDemandCap             any    `json:"onDemandCap"`
	OnDemandCapSnake        any    `json:"on_demand_cap"`
	BillingPeriodStart      string `json:"billingPeriodStart"`
	BillingPeriodStartSnake string `json:"billing_period_start"`
	BillingPeriodEnd        string `json:"billingPeriodEnd"`
	BillingPeriodEndSnake   string `json:"billing_period_end"`
}

// FetchBillingSummary fetches Grok billing with CodexBar-style fallback:
// 1) cli-chat-proxy REST billing
// 2) grok.com gRPC-web billing snapshot
func FetchBillingSummary(ctx context.Context, accessToken, proxyURL string) (*BillingSummary, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, errors.New("no access token available")
	}

	client, err := billingHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	restSummary, restErr := fetchBillingRESTWithRetry(ctx, client, accessToken)
	if restErr == nil && restSummary != nil {
		return restSummary, nil
	}

	webSummary, webErr := fetchBillingWebGRPCWithRetry(ctx, client, accessToken)
	if webErr == nil && webSummary != nil {
		return webSummary, nil
	}

	if restErr != nil {
		return nil, restErr
	}
	return nil, webErr
}

func fetchBillingRESTWithRetry(ctx context.Context, client *http.Client, accessToken string) (*BillingSummary, error) {
	summary, err := fetchBillingREST(ctx, client, accessToken)
	if err == nil || !shouldRetryBillingError(err) {
		return summary, err
	}
	return fetchBillingREST(ctx, client, accessToken)
}

func fetchBillingWebGRPCWithRetry(ctx context.Context, client *http.Client, accessToken string) (*BillingSummary, error) {
	summary, err := fetchBillingWebGRPC(ctx, client, accessToken)
	if err == nil || !shouldRetryBillingError(err) {
		return summary, err
	}
	return fetchBillingWebGRPC(ctx, client, accessToken)
}

func fetchBillingREST(ctx context.Context, client *http.Client, accessToken string) (*BillingSummary, error) {
	reqCtx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, BillingRESTEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create xAI billing request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xAI billing request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &BillingHTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var envelope billingEnvelope
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode xAI billing response: %w", err)
	}
	summary := buildBillingSummaryFromREST(envelope.Config)
	if summary == nil {
		return nil, fmt.Errorf("xAI billing response missing config")
	}
	summary.Source = "rest"
	return summary, nil
}

func fetchBillingWebGRPC(ctx context.Context, client *http.Client, accessToken string) (*BillingSummary, error) {
	reqCtx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodPost,
		BillingWebGRPCEndpoint,
		bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x00, 0x00}),
	)
	if err != nil {
		return nil, fmt.Errorf("create grok.com billing request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Origin", "https://grok.com")
	req.Header.Set("Referer", "https://grok.com/?_s=usage")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/grpc-web+proto")
	req.Header.Set("x-grpc-web", "1")
	req.Header.Set("x-user-agent", "connect-es/2.1.1")
	req.Header.Set("User-Agent", "sub2api/xai-billing")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok.com billing request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read grok.com billing response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &BillingHTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}
	if err := validateGRPCWebTrailers(body); err != nil {
		return nil, err
	}

	snapshot, err := parseGRPCWebBillingResponse(body, time.Now())
	if err != nil {
		return nil, err
	}
	summary := buildBillingSummaryFromWebGRPC(snapshot)
	if summary == nil {
		return nil, fmt.Errorf("grok.com billing response missing usage")
	}
	summary.Source = "web_grpc"
	return summary, nil
}

func buildBillingSummaryFromREST(config *billingConfig) *BillingSummary {
	if config == nil {
		return nil
	}
	monthlyLimit := parseBillingCentValue(firstNonNilBillingValue(config.MonthlyLimit, config.MonthlyLimitSnake))
	used := parseBillingCentValue(config.Used)
	onDemandCap := parseBillingCentValue(firstNonNilBillingValue(config.OnDemandCap, config.OnDemandCapSnake))
	periodStart := parseBillingTime(firstNonEmptyBillingString(config.BillingPeriodStart, config.BillingPeriodStartSnake))
	periodEnd := parseBillingTime(firstNonEmptyBillingString(config.BillingPeriodEnd, config.BillingPeriodEndSnake))

	summary := &BillingSummary{
		MonthlyLimitCents:  monthlyLimit,
		UsedCents:          used,
		OnDemandCapCents:   onDemandCap,
		BillingPeriodStart: periodStart,
		BillingPeriodEnd:   periodEnd,
	}
	if monthlyLimit != nil && *monthlyLimit > 0 && used != nil {
		usedPercent := (*used / *monthlyLimit) * 100
		summary.UsedPercent = &usedPercent
	}
	return summary
}

type webBillingSnapshot struct {
	UsedPercent *float64
	ResetsAt    *time.Time
	PeriodStart *time.Time
}

func buildBillingSummaryFromWebGRPC(snapshot *webBillingSnapshot) *BillingSummary {
	if snapshot == nil || snapshot.UsedPercent == nil {
		return nil
	}
	return &BillingSummary{
		UsedPercent:        snapshot.UsedPercent,
		BillingPeriodStart: snapshot.PeriodStart,
		BillingPeriodEnd:   snapshot.ResetsAt,
	}
}

func shouldRetryBillingError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *BillingHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusUnauthorized ||
			httpErr.StatusCode == http.StatusForbidden ||
			httpErr.StatusCode == http.StatusTooManyRequests {
			return false
		}
		if httpErr.StatusCode == http.StatusRequestTimeout ||
			httpErr.StatusCode == http.StatusBadGateway ||
			httpErr.StatusCode == http.StatusServiceUnavailable ||
			httpErr.StatusCode == http.StatusGatewayTimeout {
			return true
		}
		body := strings.ToLower(httpErr.Body)
		return strings.Contains(body, "timeout") ||
			strings.Contains(body, "deadline") ||
			strings.Contains(body, "expired") ||
			strings.Contains(body, "cancelled") ||
			strings.Contains(body, "canceled")
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline") ||
		strings.Contains(lower, "connection reset")
}

func billingHTTPClient(proxyURL string) (*http.Client, error) {
	return httppool.GetClient(httppool.Options{
		ProxyURL:              strings.TrimSpace(proxyURL),
		Timeout:               billingRequestTimeout,
		ResponseHeaderTimeout: 10 * time.Second,
	})
}

func parseBillingCentValue(value any) *float64 {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return parseBillingCentValue(v["val"])
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return &f
		}
	case float64:
		return &v
	case float32:
		f := float64(v)
		return &f
	case int:
		f := float64(v)
		return &f
	case int64:
		f := float64(v)
		return &f
	case int32:
		f := float64(v)
		return &f
	case uint:
		f := float64(v)
		return &f
	case uint64:
		f := float64(v)
		return &f
	case uint32:
		f := float64(v)
		return &f
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return &f
		}
	}
	return nil
}

func parseBillingTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, trimmed); err == nil {
			return &t
		}
	}
	return nil
}

func firstNonNilBillingValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmptyBillingString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func grpcWebDataFrames(data []byte) [][]byte {
	var frames [][]byte
	index := 0
	for index+5 <= len(data) {
		flags := data[index]
		length := int(data[index+1])<<24 | int(data[index+2])<<16 | int(data[index+3])<<8 | int(data[index+4])
		start := index + 5
		end := start + length
		if length < 0 || end > len(data) {
			break
		}
		if flags&0x80 == 0 {
			frames = append(frames, data[start:end])
		}
		index = end
	}
	return frames
}

func grpcWebTrailerFields(data []byte) map[string]string {
	fields := make(map[string]string)
	index := 0
	for index+5 <= len(data) {
		flags := data[index]
		length := int(data[index+1])<<24 | int(data[index+2])<<16 | int(data[index+3])<<8 | int(data[index+4])
		start := index + 5
		end := start + length
		if length < 0 || end > len(data) {
			break
		}
		if flags&0x80 != 0 {
			text := string(data[start:end])
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				key = strings.ToLower(strings.TrimSpace(key))
				value = strings.TrimSpace(value)
				if decoded, err := url.QueryUnescape(value); err == nil {
					value = decoded
				}
				fields[key] = value
			}
		}
		index = end
	}
	return fields
}

func validateGRPCWebTrailers(data []byte) error {
	fields := grpcWebTrailerFields(data)
	rawStatus, ok := fields["grpc-status"]
	if !ok {
		return nil
	}
	status, err := strconv.Atoi(strings.TrimSpace(rawStatus))
	if err != nil || status == 0 {
		return nil
	}
	return fmt.Errorf("grok.com billing RPC failed with status %d: %s", status, fields["grpc-message"])
}

type protobufScan struct {
	fixed32Fields []protobufFixed32Field
	varintFields  []protobufVarintField
}

type protobufFixed32Field struct {
	path  []uint64
	value float32
	order int
}

type protobufVarintField struct {
	path  []uint64
	value uint64
}

func parseGRPCWebBillingResponse(data []byte, now time.Time) (*webBillingSnapshot, error) {
	payloads := grpcWebDataFrames(data)
	if len(payloads) == 0 {
		return nil, errors.New("grok.com billing returned no protobuf payload")
	}

	scan := protobufScan{}
	order := 0
	for _, payload := range payloads {
		nested, nestedOrder := scanProtobuf(payload, 0, nil, order)
		scan.fixed32Fields = append(scan.fixed32Fields, nested.fixed32Fields...)
		scan.varintFields = append(scan.varintFields, nested.varintFields...)
		order = nestedOrder
	}

	var parsedPercent *float64
	bestOrder := math.MaxInt32
	for _, field := range scan.fixed32Fields {
		if len(field.path) == 0 || field.path[len(field.path)-1] != 1 {
			continue
		}
		value := float64(field.value)
		if !isFinitePercent(value) {
			continue
		}
		if field.order < bestOrder {
			bestOrder = field.order
			v := value
			parsedPercent = &v
		}
	}

	type resetCandidate struct {
		path []uint64
		date time.Time
	}
	var resetCandidates []resetCandidate
	for _, field := range scan.varintFields {
		if field.value < 1_700_000_000 || field.value > 2_100_000_000 {
			continue
		}
		resetCandidates = append(resetCandidates, resetCandidate{
			path: field.path,
			date: time.Unix(int64(field.value), 0).UTC(),
		})
	}

	var resetsAt *time.Time
	var periodStart *time.Time
	futureResets := make([]resetCandidate, 0, len(resetCandidates))
	for _, candidate := range resetCandidates {
		if candidate.date.After(now) {
			futureResets = append(futureResets, candidate)
		}
	}
	for _, candidate := range resetCandidates {
		if pathEqual(candidate.path, []uint64{1, 4, 1}) {
			t := candidate.date
			periodStart = &t
		}
	}
	for _, candidate := range futureResets {
		if pathEqual(candidate.path, []uint64{1, 5, 1}) {
			t := candidate.date
			if resetsAt == nil || candidate.date.Before(*resetsAt) {
				resetsAt = &t
			}
		}
	}
	if resetsAt == nil {
		for _, candidate := range futureResets {
			t := candidate.date
			if resetsAt == nil || candidate.date.Before(*resetsAt) {
				resetsAt = &t
			}
		}
	}

	noUsageYet := parsedPercent == nil &&
		len(scan.fixed32Fields) == 0 &&
		resetsAt != nil &&
		hasPathPrefix(scan.varintFields, []uint64{1, 6})
	if parsedPercent == nil && noUsageYet {
		zero := 0.0
		parsedPercent = &zero
	}
	if parsedPercent == nil {
		return nil, errors.New("could not parse grok.com billing usage")
	}

	return &webBillingSnapshot{
		UsedPercent: parsedPercent,
		ResetsAt:    resetsAt,
		PeriodStart: periodStart,
	}, nil
}

func scanProtobuf(data []byte, depth int, path []uint64, order int) (protobufScan, int) {
	scan := protobufScan{}
	index := 0
	nextOrder := order

	for index < len(data) {
		fieldStart := index
		key, ok := readProtobufVarint(data, &index)
		if !ok || key == 0 {
			index = fieldStart + 1
			continue
		}
		fieldNumber := key >> 3
		wireType := key & 0x07
		fieldPath := append(path, fieldNumber)

		switch wireType {
		case 0:
			value, ok := readProtobufVarint(data, &index)
			if !ok {
				index = fieldStart + 1
				continue
			}
			scan.varintFields = append(scan.varintFields, protobufVarintField{
				path:  append([]uint64(nil), fieldPath...),
				value: value,
			})
		case 1:
			if index+8 > len(data) {
				return scan, nextOrder
			}
			index += 8
		case 2:
			length, ok := readProtobufVarint(data, &index)
			if !ok || length > uint64(len(data)-index) {
				index = fieldStart + 1
				continue
			}
			start := index
			end := index + int(length)
			if depth < 4 {
				nested, nestedOrder := scanProtobuf(data[start:end], depth+1, fieldPath, nextOrder)
				scan.fixed32Fields = append(scan.fixed32Fields, nested.fixed32Fields...)
				scan.varintFields = append(scan.varintFields, nested.varintFields...)
				nextOrder = nestedOrder
			}
			index = end
		case 5:
			if index+4 > len(data) {
				return scan, nextOrder
			}
			bits := binary.LittleEndian.Uint32(data[index : index+4])
			scan.fixed32Fields = append(scan.fixed32Fields, protobufFixed32Field{
				path:  append([]uint64(nil), fieldPath...),
				value: math.Float32frombits(bits),
				order: nextOrder,
			})
			nextOrder++
			index += 4
		default:
			index = fieldStart + 1
		}
	}

	return scan, nextOrder
}

func readProtobufVarint(data []byte, index *int) (uint64, bool) {
	var value uint64
	var shift uint
	for *index < len(data) && shift < 64 {
		byteValue := data[*index]
		*index++
		value |= uint64(byteValue&0x7F) << shift
		if byteValue&0x80 == 0 {
			return value, true
		}
		shift += 7
	}
	return 0, false
}

func isFinitePercent(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 100
}

func pathEqual(lhs, rhs []uint64) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	for i := range lhs {
		if lhs[i] != rhs[i] {
			return false
		}
	}
	return true
}

func hasPathPrefix(fields []protobufVarintField, prefix []uint64) bool {
	for _, field := range fields {
		if len(field.path) < len(prefix) {
			continue
		}
		if pathEqual(field.path[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

// ClassifyBillingError maps billing fetch failures to machine-readable codes.
func ClassifyBillingError(err error) string {
	var httpErr *BillingHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusUnauthorized:
			return "unauthenticated"
		case http.StatusForbidden:
			return "forbidden"
		case http.StatusTooManyRequests:
			return "rate_limited"
		}
	}
	return "network_error"
}
