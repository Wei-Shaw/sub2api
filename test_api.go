package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIConfig represents the API configuration format
type APIConfig struct {
	Type string `json:"_type"`
	Key  string `json:"key"`
	URL  string `json:"url"`
}

// TestPayload represents the Claude API request
type TestPayload struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	Messages    []Message     `json:"messages"`
	System      string        `json:"system,omitempty"`
	Stream      bool          `json:"stream"`
}

// Message represents a message in the request
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func main() {
	// Parse command line flags
	configJSON := flag.String("config", `{"_type":"newapi_channel_conn","key":"sk-jH6fMY21y9BgyuOg9guEcch3BnGnnGJhVkUhNLb9yMzU20be","url":"https://nexaxis.ai"}`, "API configuration in JSON format")
	model := flag.String("model", "claude-3-5-sonnet-20241022", "Model to test")
	timeout := flag.Duration("timeout", 30*time.Second, "Request timeout")
	flag.Parse()

	// Parse config
	var config APIConfig
	if err := json.Unmarshal([]byte(*configJSON), &config); err != nil {
		fmt.Printf("❌ Failed to parse config: %v\n", err)
		return
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("🔍 Claude API Configuration Validation")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Type: %s\n", config.Type)
	fmt.Printf("URL:  %s\n", config.URL)
	fmt.Printf("Key:  %s...%s\n", config.Key[:10], config.Key[len(config.Key)-10:])
	fmt.Printf("Model: %s\n", *model)
	fmt.Println(strings.Repeat("-", 60))

	// Validate configuration
	if err := validateConfig(config); err != nil {
		fmt.Printf("❌ Configuration validation failed: %v\n", err)
		return
	}

	// Test API connection
	fmt.Println("\n📡 Testing API connection...")
	if err := testAPIConnection(config, *model, *timeout); err != nil {
		fmt.Printf("❌ API test failed: %v\n", err)
		return
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ All checks passed! API appears to be valid.")
	fmt.Println(strings.Repeat("=", 60))
}

func validateConfig(config APIConfig) error {
	if config.Type == "" {
		return fmt.Errorf("_type is empty")
	}
	if config.Key == "" {
		return fmt.Errorf("key is empty")
	}
	if config.URL == "" {
		return fmt.Errorf("url is empty")
	}
	if !strings.HasPrefix(config.Key, "sk-") {
		return fmt.Errorf("key should start with 'sk-'")
	}
	if !strings.HasPrefix(config.URL, "http://") && !strings.HasPrefix(config.URL, "https://") {
		return fmt.Errorf("url should start with http:// or https://")
	}
	fmt.Println("✓ Configuration format is valid")
	return nil
}

func testAPIConnection(config APIConfig, model string, timeout time.Duration) error {
	// Prepare request
	payload := TestPayload{
		Model:     model,
		MaxTokens: 50,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Say 'hello' in one word",
			},
		},
		Stream: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Build URL - construct the API endpoint
	apiURL := strings.TrimSuffix(config.URL, "/") + "/v1/messages"
	fmt.Printf("Testing endpoint: %s\n", apiURL)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Create request
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (standard Claude API headers)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.Key))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", config.Key) // Some proxies use this instead

	fmt.Println("Headers set:")
	fmt.Printf("  - Content-Type: application/json\n")
	fmt.Printf("  - Authorization: Bearer %s...%s\n", config.Key[:10], config.Key[len(config.Key)-10:])
	fmt.Printf("  - anthropic-version: 2023-06-01\n")

	// Send request
	fmt.Println("\n📨 Sending request...")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

	// Parse response
	var responseData map[string]interface{}
	if err := json.Unmarshal(responseBody, &responseData); err != nil {
		fmt.Printf("Response (raw): %s\n", string(responseBody)[:min(200, len(responseBody))])
	} else {
		responseJSON, _ := json.MarshalIndent(responseData, "", "  ")
		fmt.Printf("Response (JSON):\n%s\n", string(responseJSON)[:min(300, len(responseJSON))])
	}

	// Check status
	switch resp.StatusCode {
	case http.StatusOK:
		fmt.Println("✓ Successfully connected and received response")
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		if respStr := string(responseBody); strings.Contains(respStr, "invalid") || strings.Contains(respStr, "unauthorized") {
			return fmt.Errorf("authentication failed - invalid API key or URL")
		}
		return fmt.Errorf("authorization error (status %d)", resp.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("endpoint not found (status 404) - the URL or API path may be incorrect")
	case http.StatusBadRequest:
		return fmt.Errorf("bad request (status 400) - check the payload format")
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limit exceeded (status 429)")
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("server error (status %d)", resp.StatusCode)
		}
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a APIConfig) String() string {
	return fmt.Sprintf("APIConfig{Type:%s, Key:%s...%s, URL:%s}",
		a.Type, a.Key[:10], a.Key[len(a.Key)-10:], a.URL)
}
