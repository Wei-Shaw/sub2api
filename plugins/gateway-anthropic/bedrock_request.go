package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// --- Bedrock constants ---

const defaultBedrockRegion = "us-east-1"

// bedrockCrossRegionPrefixes lists all Bedrock cross-region inference prefixes.
var bedrockCrossRegionPrefixes = []string{
	"us.", "eu.", "apac.", "jp.", "au.", "us-gov.", "global.",
}

// defaultBedrockModelMapping maps standard Claude model names to Bedrock model IDs.
// The "us." prefix is the default; resolveBedrockModel adjusts it per account region.
var defaultBedrockModelMapping = map[string]string{
	// Claude Opus
	"claude-opus-4-7":          "us.anthropic.claude-opus-4-7-v1",
	"claude-opus-4-6-thinking": "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-6":          "us.anthropic.claude-opus-4-6-v1",
	"claude-opus-4-5-thinking": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-5-20251101": "us.anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-1":          "us.anthropic.claude-opus-4-1-20250805-v1:0",
	"claude-opus-4-20250514":   "us.anthropic.claude-opus-4-20250514-v1:0",
	// Claude Sonnet
	"claude-sonnet-4-6-thinking": "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-6":          "us.anthropic.claude-sonnet-4-6",
	"claude-sonnet-4-5":          "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-thinking": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-20250929": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-20250514":   "us.anthropic.claude-sonnet-4-20250514-v1:0",
	// Claude Haiku
	"claude-haiku-4-5":          "us.anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-haiku-4-5-20251001": "us.anthropic.claude-haiku-4-5-20251001-v1:0",
}

// --- Bedrock URL building ---

// buildBedrockURL constructs the Bedrock InvokeModel endpoint URL.
// stream=true uses the invoke-with-response-stream endpoint.
func buildBedrockURL(region, modelID string, stream bool) string {
	if region == "" {
		region = defaultBedrockRegion
	}
	encoded := url.PathEscape(modelID)
	// url.PathEscape does not encode colons; Bedrock expects %3A.
	encoded = strings.ReplaceAll(encoded, ":", "%3A")
	if stream {
		return fmt.Sprintf(
			"https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke-with-response-stream",
			region, encoded,
		)
	}
	return fmt.Sprintf(
		"https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke",
		region, encoded,
	)
}

// --- Bedrock model resolution ---

// bedrockCrossRegionPrefix returns the inference profile prefix for a region.
func bedrockCrossRegionPrefix(region string) string {
	switch {
	case strings.HasPrefix(region, "us-gov"):
		return "us-gov"
	case strings.HasPrefix(region, "us-"):
		return "us"
	case strings.HasPrefix(region, "eu-"):
		return "eu"
	case region == "ap-northeast-1":
		return "jp"
	case region == "ap-southeast-2":
		return "au"
	case strings.HasPrefix(region, "ap-"):
		return "apac"
	case strings.HasPrefix(region, "ca-"), strings.HasPrefix(region, "sa-"):
		return "us"
	default:
		return "us"
	}
}

// adjustBedrockModelRegionPrefix replaces the cross-region prefix in a model
// ID to match the target region. "global" forces the global. prefix.
func adjustBedrockModelRegionPrefix(modelID, region string) string {
	targetPrefix := bedrockCrossRegionPrefix(region)
	if region == "global" {
		targetPrefix = "global"
	}
	for _, p := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, p) {
			if p == targetPrefix+"." {
				return modelID
			}
			return targetPrefix + "." + modelID[len(p):]
		}
	}
	return modelID
}

func isRegionalBedrockModelID(modelID string) bool {
	for _, prefix := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

func isLikelyBedrockModelID(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "arn:") {
		return true
	}
	for _, prefix := range []string{
		"anthropic.", "amazon.", "meta.", "mistral.", "cohere.",
		"ai21.", "deepseek.", "stability.", "writer.", "nova.",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return isRegionalBedrockModelID(lower)
}

// resolveBedrockModel resolves a requested model to a Bedrock model ID,
// applying account model_mapping, default mapping, and region prefix adjustment.
func resolveBedrockModel(requestedModel string, credentialsJSON []byte) (string, bool) {
	creds := parseBedrockCredentialsRaw(credentialsJSON)

	// Apply account-level model_mapping first.
	mappedModel := requestedModel
	if mm := extractModelMapping(credentialsJSON); len(mm) > 0 {
		if m, ok := mm[requestedModel]; ok {
			mappedModel = m
		}
	}

	// Normalise to Bedrock model ID.
	modelID, shouldAdjust, ok := normalizeBedrockModelID(mappedModel)
	if !ok {
		return "", false
	}

	// Adjust region prefix.
	if shouldAdjust {
		region := creds.region
		if region == "" {
			region = defaultBedrockRegion
		}
		if creds.forceGlobal {
			region = "global"
		}
		modelID = adjustBedrockModelRegionPrefix(modelID, region)
	}
	return modelID, true
}

func normalizeBedrockModelID(modelID string) (normalized string, shouldAdjustRegion bool, ok bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", false, false
	}
	if mapped, exists := defaultBedrockModelMapping[modelID]; exists {
		return mapped, true, true
	}
	if isRegionalBedrockModelID(modelID) {
		return modelID, true, true
	}
	if isLikelyBedrockModelID(modelID) {
		return modelID, false, true
	}
	return "", false, false
}

// --- Bedrock credentials ---

type bedrockCreds struct {
	apiKey          string // Bedrock cross-region API Key (Bearer token)
	accessKeyID     string // AWS SigV4 access key
	secretAccessKey string // AWS SigV4 secret key
	sessionToken    string // AWS STS session token (optional)
	region          string
	forceGlobal     bool
}

// parseBedrockCredentialsRaw extracts Bedrock credential fields from raw JSON.
func parseBedrockCredentialsRaw(credentialsJSON []byte) *bedrockCreds {
	if len(credentialsJSON) == 0 {
		return &bedrockCreds{region: defaultBedrockRegion}
	}
	var raw map[string]any
	if err := json.Unmarshal(credentialsJSON, &raw); err != nil {
		return &bedrockCreds{region: defaultBedrockRegion}
	}
	str := func(key string) string {
		v, _ := raw[key].(string)
		return strings.TrimSpace(v)
	}
	c := &bedrockCreds{
		apiKey:          str("api_key"),
		accessKeyID:     str("aws_access_key_id"),
		secretAccessKey: str("aws_secret_access_key"),
		sessionToken:    str("aws_session_token"),
		region:          str("aws_region"),
		forceGlobal:     str("aws_force_global") == "true",
	}
	if c.region == "" {
		c.region = defaultBedrockRegion
	}
	return c
}

// isBedrockAPIKey returns true when the account uses API Key auth (not SigV4).
func (c *bedrockCreds) isAPIKey() bool {
	return c.apiKey != ""
}

// --- Bedrock request body preparation ---

// prepareBedrockRequestBody transforms the request body for Bedrock:
//  1. Injects anthropic_version (required by Bedrock)
//  2. Injects anthropic_beta tokens into the body
//  3. Removes model field (Bedrock uses URL)
//  4. Removes stream field (Bedrock uses endpoint)
//  5. Cleans up Bedrock-unsupported fields
func prepareBedrockRequestBody(body []byte, betaHeader string) ([]byte, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse request body: %w", err)
	}

	// 1. Inject anthropic_version.
	versionBytes, _ := json.Marshal("bedrock-2023-05-31")
	parsed["anthropic_version"] = versionBytes

	// 2. Inject anthropic_beta.
	betaTokens := resolveBedrockBetaTokens(betaHeader, body)
	if len(betaTokens) > 0 {
		tokensBytes, _ := json.Marshal(betaTokens)
		parsed["anthropic_beta"] = tokensBytes
	}

	// 3. Remove model (Bedrock specifies via URL).
	delete(parsed, "model")

	// 4. Remove stream (Bedrock uses endpoint path).
	delete(parsed, "stream")

	// 5. Remove output_config (not supported by Bedrock).
	delete(parsed, "output_config")

	result, err := json.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("marshal bedrock body: %w", err)
	}
	return result, nil
}

// --- Bedrock beta token resolution ---

// bedrockSupportedBetaTokens is the whitelist of beta tokens Bedrock accepts.
var bedrockSupportedBetaTokens = map[string]bool{
	"computer-use-2025-01-24":         true,
	"computer-use-2025-11-24":         true,
	"context-1m-2025-08-07":           true,
	"context-management-2025-06-27":   true,
	"compact-2026-01-12":              true,
	"interleaved-thinking-2025-05-14": true,
	"tool-search-tool-2025-10-19":     true,
	"tool-examples-2025-10-29":        true,
}

// bedrockBetaTokenTransforms maps Anthropic API beta tokens to Bedrock equivalents.
var bedrockBetaTokenTransforms = map[string]string{
	"advanced-tool-use-2025-11-20": "tool-search-tool-2025-10-19",
}

// resolveBedrockBetaTokens computes the Bedrock beta token list from the
// client header and request body.
func resolveBedrockBetaTokens(betaHeader string, body []byte) []string {
	tokens := parseBedrockBetaHeader(betaHeader)
	tokens = autoInjectBedrockBetaTokens(tokens, body)
	return filterBedrockBetaTokens(tokens)
}

func parseBedrockBetaHeader(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	var tokens []string
	for _, part := range strings.Split(header, ",") {
		t := strings.TrimSpace(part)
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// autoInjectBedrockBetaTokens detects request body features and injects
// required beta tokens that clients may not include in headers.
func autoInjectBedrockBetaTokens(tokens []string, body []byte) []string {
	seen := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		seen[t] = true
	}
	inject := func(token string) {
		if !seen[token] {
			tokens = append(tokens, token)
			seen[token] = true
		}
	}

	// Check for thinking field -> needs interleaved-thinking beta.
	var parsed map[string]json.RawMessage
	if json.Unmarshal(body, &parsed) != nil {
		return tokens
	}
	if _, ok := parsed["thinking"]; ok {
		inject("interleaved-thinking-2025-05-14")
	}

	return tokens
}

func filterBedrockBetaTokens(tokens []string) []string {
	seen := make(map[string]bool, len(tokens))
	var result []string
	for _, t := range tokens {
		if replacement, ok := bedrockBetaTokenTransforms[t]; ok {
			t = replacement
		}
		if bedrockSupportedBetaTokens[t] && !seen[t] {
			result = append(result, t)
			seen[t] = true
		}
	}
	// Auto-associate: tool-search-tool requires tool-examples.
	if seen["tool-search-tool-2025-10-19"] && !seen["tool-examples-2025-10-29"] {
		result = append(result, "tool-examples-2025-10-29")
	}
	return result
}

// --- AWS SigV4 signing (no AWS SDK dependency) ---

// signBedrockRequest signs an HTTP request with AWS SigV4 for Bedrock.
func signBedrockRequest(
	ctx context.Context,
	req *http.Request,
	body []byte,
	creds *bedrockCreds,
) error {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzDate)
	if creds.sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.sessionToken)
	}
	req.Header.Set("Host", req.URL.Host)

	payloadHash := sha256Hex(body)

	// 1. Canonical request.
	canonicalHeaders, signedHeaders := buildCanonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQueryString(req.URL),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	// 2. String to sign.
	credentialScope := dateStamp + "/" + creds.region + "/bedrock/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// 3. Signing key.
	signingKey := deriveSigningKey(creds.secretAccessKey, dateStamp, creds.region, "bedrock")

	// 4. Signature.
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// 5. Authorization header.
	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.accessKeyID, credentialScope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authHeader)

	return nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func canonicalURI(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQueryString(u *url.URL) string {
	params := u.Query()
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		for _, v := range params[k] {
			pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}

func buildCanonicalHeaders(req *http.Request) (canonicalHeaders, signedHeaders string) {
	// Collect headers to sign: content-type, host, x-amz-*.
	type hdr struct {
		key string
		val string
	}
	var headers []hdr

	addHeader := func(key string) {
		lower := strings.ToLower(key)
		val := strings.TrimSpace(req.Header.Get(key))
		if val != "" {
			headers = append(headers, hdr{lower, val})
		}
	}

	addHeader("Content-Type")
	addHeader("Host")
	for key := range req.Header {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-amz-") {
			headers = append(headers, hdr{lower, strings.TrimSpace(req.Header.Get(key))})
		}
	}

	sort.Slice(headers, func(i, j int) bool {
		return headers[i].key < headers[j].key
	})

	var canonParts []string
	var signedParts []string
	seen := make(map[string]bool)
	for _, h := range headers {
		if seen[h.key] {
			continue
		}
		seen[h.key] = true
		canonParts = append(canonParts, h.key+":"+h.val+"\n")
		signedParts = append(signedParts, h.key)
	}

	return strings.Join(canonParts, ""), strings.Join(signedParts, ";")
}

// claudeVersionRe matches Claude model version numbers for feature detection.
var claudeVersionRe = regexp.MustCompile(`claude-(?:haiku|sonnet|opus)-(\d+)[-.](\d+)`)

// isBedrockClaude45OrNewer detects Claude 4.5+ for cache_control.ttl support.
func isBedrockClaude45OrNewer(modelID string) bool {
	matches := claudeVersionRe.FindStringSubmatch(strings.ToLower(modelID))
	if matches == nil {
		return false
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	return major > 4 || (major == 4 && minor >= 5)
}