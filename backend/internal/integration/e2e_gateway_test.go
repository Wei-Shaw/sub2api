//go:build e2e

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	baseURL = getEnv("BASE_URL", "http://localhost:8080")
	// ENDPOINT_PREFIX: 端点前缀，支持混合模式和非混合模式测试
	// - "" (默认): 使用 /v1/messages, /v1beta/models（混合模式，可调度 antigravity 账户）
	// - "/antigravity": 使用 /antigravity/v1/messages, /antigravity/v1beta/models（非混合模式，仅 antigravity 账户）
	endpointPrefix = getEnv("ENDPOINT_PREFIX", "")
	testInterval   = 1 * time.Second // 测试间隔，防止限流
)

const (
	// 注意：E2E 测试请使用环境变量注入密钥，避免任何凭证进入仓库历史。
	// 例如：
	//   export CLAUDE_API_KEY="sk-..."
	//   export GEMINI_API_KEY="sk-..."
	claudeAPIKeyEnv = "CLAUDE_API_KEY"
	geminiAPIKeyEnv = "GEMINI_API_KEY"
)

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// Claude 模型列表
var claudeModels = []string{
	// Opus 系列
	"claude-opus-4-5-thinking", // 直接支持
	"claude-opus-4",            // 映射到 claude-opus-4-5-thinking
	"claude-opus-4-5-20251101", // 映射到 claude-opus-4-5-thinking
	// Sonnet 系列
	"claude-sonnet-4-5",          // 直接支持
	"claude-sonnet-4-5-thinking", // 直接支持
	"claude-sonnet-4-5-20250929", // 映射到 claude-sonnet-4-5-thinking
	"claude-3-5-sonnet-20241022", // 映射到 claude-sonnet-4-5
	// Haiku 系列（映射到 gemini-3-flash）
	"claude-haiku-4",
	"claude-haiku-4-5",
	"claude-haiku-4-5-20251001",
	"claude-3-haiku-20240307",
}

// Gemini 模型列表
var geminiModels = []string{
	"gemini-2.5-flash",
	"gemini-2.5-flash-lite",
	"gemini-3-flash",
	"gemini-3-pro-low",
	"gemini-3-pro-high",
}

func TestMain(m *testing.M) {
	mode := "混合模式"
	if endpointPrefix != "" {
		mode = "Antigravity 模式"
	}
	claudeKeySet := strings.TrimSpace(os.Getenv(claudeAPIKeyEnv)) != ""
	geminiKeySet := strings.TrimSpace(os.Getenv(geminiAPIKeyEnv)) != ""
	fmt.Printf("\n🚀 E2E Gateway Tests - %s (prefix=%q, %s, %s=%v, %s=%v)\n\n",
		baseURL,
		endpointPrefix,
		mode,
		claudeAPIKeyEnv,
		claudeKeySet,
		geminiAPIKeyEnv,
		geminiKeySet,
	)
	os.Exit(m.Run())
}

func requireClaudeAPIKey(t *testing.T) string {
	t.Helper()
	key := strings.TrimSpace(os.Getenv(claudeAPIKeyEnv))
	if key == "" {
		t.Skipf("未设置 %s，跳过 Claude 相关 E2E 测试", claudeAPIKeyEnv)
	}
	return key
}

func requireGeminiAPIKey(t *testing.T) string {
	t.Helper()
	key := strings.TrimSpace(os.Getenv(geminiAPIKeyEnv))
	if key == "" {
		t.Skipf("未设置 %s，跳过 Gemini 相关 E2E 测试", geminiAPIKeyEnv)
	}
	return key
}

// TestClaudeModelsList 测试 GET /v1/models
func TestClaudeModelsList(t *testing.T) {
	claudeKey := requireClaudeAPIKey(t)
	url := baseURL + endpointPrefix + "/v1/models"

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+claudeKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if result["object"] != "list" {
		t.Errorf("期望 object=list, 得到 %v", result["object"])
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("响应缺少 data 数组")
	}
	t.Logf("✅ 返回 %d 个模型", len(data))
}

// TestGeminiModelsList 测试 GET /v1beta/models
func TestGeminiModelsList(t *testing.T) {
	geminiKey := requireGeminiAPIKey(t)
	url := baseURL + endpointPrefix + "/v1beta/models"

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+geminiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	models, ok := result["models"].([]any)
	if !ok {
		t.Fatal("响应缺少 models 数组")
	}
	t.Logf("✅ 返回 %d 个模型", len(models))
}

// TestClaudeMessages 测试 Claude /v1/messages 接口
func TestClaudeMessages(t *testing.T) {
	claudeKey := requireClaudeAPIKey(t)
	for i, model := range claudeModels {
		if i > 0 {
			time.Sleep(testInterval)
		}
		t.Run(model+"_非流式", func(t *testing.T) {
			testClaudeMessage(t, claudeKey, model, false)
		})
		time.Sleep(testInterval)
		t.Run(model+"_流式", func(t *testing.T) {
			testClaudeMessage(t, claudeKey, model, true)
		})
	}
}

func testClaudeMessage(t *testing.T, claudeKey string, model string, stream bool) {
	url := baseURL + endpointPrefix + "/v1/messages"

	payload := map[string]any{
		"model":      model,
		"max_tokens": 50,
		"stream":     stream,
		"messages": []map[string]string{
			{"role": "user", "content": "Say 'hello' in one word."},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+claudeKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if stream {
		// 流式：读取 SSE 事件
		scanner := bufio.NewScanner(resp.Body)
		eventCount := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				eventCount++
				if eventCount >= 3 {
					break
				}
			}
		}
		if eventCount == 0 {
			t.Fatal("未收到任何 SSE 事件")
		}
		t.Logf("✅ 收到 %d+ 个 SSE 事件", eventCount)
	} else {
		// 非流式：解析 JSON 响应
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if result["type"] != "message" {
			t.Errorf("期望 type=message, 得到 %v", result["type"])
		}
		t.Logf("✅ 收到消息响应 id=%v", result["id"])
	}
}

// TestGeminiGenerateContent 测试 Gemini /v1beta/models/:model 接口
func TestGeminiGenerateContent(t *testing.T) {
	geminiKey := requireGeminiAPIKey(t)
	for i, model := range geminiModels {
		if i > 0 {
			time.Sleep(testInterval)
		}
		t.Run(model+"_非流式", func(t *testing.T) {
			testGeminiGenerate(t, geminiKey, model, false)
		})
		time.Sleep(testInterval)
		t.Run(model+"_流式", func(t *testing.T) {
			testGeminiGenerate(t, geminiKey, model, true)
		})
	}
}

func testGeminiGenerate(t *testing.T, geminiKey string, model string, stream bool) {
	action := "generateContent"
	if stream {
		action = "streamGenerateContent"
	}
	url := fmt.Sprintf("%s%s/v1beta/models/%s:%s", baseURL, endpointPrefix, model, action)
	if stream {
		url += "?alt=sse"
	}

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": "Say 'hello' in one word."},
				},
			},
		},
		"generationConfig": map[string]int{
			"maxOutputTokens": 50,
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+geminiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if stream {
		// 流式：读取 SSE 事件
		scanner := bufio.NewScanner(resp.Body)
		eventCount := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				eventCount++
				if eventCount >= 3 {
					break
				}
			}
		}
		if eventCount == 0 {
			t.Fatal("未收到任何 SSE 事件")
		}
		t.Logf("✅ 收到 %d+ 个 SSE 事件", eventCount)
	} else {
		// 非流式：解析 JSON 响应
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if _, ok := result["candidates"]; !ok {
			t.Error("响应缺少 candidates 字段")
		}
		t.Log("✅ 收到 candidates 响应")
	}
}

// TestClaudeMessagesWithComplexTools 测试带复杂工具 schema 的请求
// 模拟 Claude Code 发送的请求，包含需要清理的 JSON Schema 字段
func TestClaudeMessagesWithComplexTools(t *testing.T) {
	claudeKey := requireClaudeAPIKey(t)
	// 测试模型列表（只测试几个代表性模型）
	models := []string{
		"claude-opus-4-5-20251101",  // Claude 模型
		"claude-haiku-4-5-20251001", // 映射到 Gemini
	}

	for i, model := range models {
		if i > 0 {
			time.Sleep(testInterval)
		}
		t.Run(model+"_复杂工具", func(t *testing.T) {
			testClaudeMessageWithTools(t, claudeKey, model)
		})
	}
}

func testClaudeMessageWithTools(t *testing.T, claudeKey string, model string) {
	url := baseURL + endpointPrefix + "/v1/messages"

	// 构造包含复杂 schema 的工具定义（模拟 Claude Code 的工具）
	// 这些字段需要被 cleanJSONSchema 清理
	tools := []map[string]any{
		{
			"name":        "read_file",
			"description": "Read file contents",
			"input_schema": map[string]any{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type":    "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "File path",
						"minLength":   1,
						"maxLength":   4096,
						"pattern":     "^[^\\x00]+$",
					},
					"encoding": map[string]any{
						"type":    []string{"string", "null"},
						"default": "utf-8",
						"enum":    []string{"utf-8", "ascii", "latin-1"},
					},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "write_file",
			"description": "Write content to file",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
					"content": map[string]any{
						"type":      "string",
						"maxLength": 1048576,
					},
				},
				"required":             []string{"path", "content"},
				"additionalProperties": false,
				"strict":               true,
			},
		},
		{
			"name":        "list_files",
			"description": "List files in directory",
			"input_schema": map[string]any{
				"$id":  "https://example.com/list-files.schema.json",
				"type": "object",
				"properties": map[string]any{
					"directory": map[string]any{
						"type": "string",
					},
					"patterns": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":      "string",
							"minLength": 1,
						},
						"minItems":    1,
						"maxItems":    100,
						"uniqueItems": true,
					},
					"recursive": map[string]any{
						"type":    "boolean",
						"default": false,
					},
				},
				"required":             []string{"directory"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "search_code",
			"description": "Search code in files",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":      "string",
						"minLength": 1,
						"format":    "regex",
					},
					"max_results": map[string]any{
						"type":             "integer",
						"minimum":          1,
						"maximum":          1000,
						"exclusiveMinimum": 0,
						"default":          100,
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
				"examples": []map[string]any{
					{"query": "function.*test", "max_results": 50},
				},
			},
		},
		// 测试 required 引用不存在的属性（应被自动过滤）
		{
			"name":        "invalid_required_tool",
			"description": "Tool with invalid required field",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
				},
				// "nonexistent_field" 不存在于 properties 中，应被过滤掉
				"required": []string{"name", "nonexistent_field"},
			},
		},
		// 测试没有 properties 的 schema（应自动添加空 properties）
		{
			"name":        "no_properties_tool",
			"description": "Tool without properties",
			"input_schema": map[string]any{
				"type":     "object",
				"required": []string{"should_be_removed"},
			},
		},
		// 测试没有 type 的 schema（应自动添加 type: OBJECT）
		{
			"name":        "no_type_tool",
			"description": "Tool without type",
			"input_schema": map[string]any{
				"properties": map[string]any{
					"value": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": 100,
		"stream":     false,
		"messages": []map[string]string{
			{"role": "user", "content": "List files in the current directory"},
		},
		"tools": tools,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+claudeKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// 400 错误说明 schema 清理不完整
	if resp.StatusCode == 400 {
		t.Fatalf("Schema 清理失败，收到 400 错误: %s", string(respBody))
	}

	// 503 可能是账号限流，不算测试失败
	if resp.StatusCode == 503 {
		t.Skipf("账号暂时不可用 (503): %s", string(respBody))
	}

	// 429 是限流
	if resp.StatusCode == 429 {
		t.Skipf("请求被限流 (429): %s", string(respBody))
	}

	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if result["type"] != "message" {
		t.Errorf("期望 type=message, 得到 %v", result["type"])
	}
	t.Logf("✅ 复杂工具 schema 测试通过, id=%v", result["id"])
}

// TestClaudeMessagesWithThinkingAndTools 测试 thinking 模式下带工具调用的场景
// 验证：当历史 assistant 消息包含 tool_use 但没有 signature 时，
// 系统应自动添加 dummy thought_signature 避免 Gemini 400 错误
func TestClaudeMessagesWithThinkingAndTools(t *testing.T) {
	claudeKey := requireClaudeAPIKey(t)
	models := []string{
		"claude-haiku-4-5-20251001", // gemini-3-flash
	}
	for i, model := range models {
		if i > 0 {
			time.Sleep(testInterval)
		}
		t.Run(model+"_thinking模式工具调用", func(t *testing.T) {
			testClaudeThinkingWithToolHistory(t, claudeKey, model)
		})
	}
}

func testClaudeThinkingWithToolHistory(t *testing.T, claudeKey string, model string) {
	url := baseURL + endpointPrefix + "/v1/messages"

	// 模拟历史对话：用户请求 → assistant 调用工具 → 工具返回 → 继续对话
	// 注意：tool_use 块故意不包含 signature，测试系统是否能正确添加 dummy signature
	payload := map[string]any{
		"model":      model,
		"max_tokens": 200,
		"stream":     false,
		// 开启 thinking 模式
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 1024,
		},
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "List files in the current directory",
			},
			// assistant 消息包含 tool_use 但没有 signature
			map[string]any{
				"role": "assistant",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "I'll list the files for you.",
					},
					{
						"type":  "tool_use",
						"id":    "toolu_01XGmNv",
						"name":  "Bash",
						"input": map[string]any{"command": "ls -la"},
						// 故意不包含 signature
					},
				},
			},
			// 工具结果
			map[string]any{
				"role": "user",
				"content": []map[string]any{
					{
						"type":        "tool_result",
						"tool_use_id": "toolu_01XGmNv",
						"content":     "file1.txt\nfile2.txt\ndir1/",
					},
				},
			},
		},
		"tools": []map[string]any{
			{
				"name":        "Bash",
				"description": "Execute bash commands",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type": "string",
						},
					},
					"required": []string{"command"},
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+claudeKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// 400 错误说明 thought_signature 处理失败
	if resp.StatusCode == 400 {
		t.Fatalf("thought_signature 处理失败，收到 400 错误: %s", string(respBody))
	}

	// 503 可能是账号限流，不算测试失败
	if resp.StatusCode == 503 {
		t.Skipf("账号暂时不可用 (503): %s", string(respBody))
	}

	// 429 是限流
	if resp.StatusCode == 429 {
		t.Skipf("请求被限流 (429): %s", string(respBody))
	}

	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if result["type"] != "message" {
		t.Errorf("期望 type=message, 得到 %v", result["type"])
	}
	t.Logf("✅ thinking 模式工具调用测试通过, id=%v", result["id"])
}

// TestClaudeMessagesWithGeminiModel 测试在 Claude 端点使用 Gemini 模型
// 验证：通过 /v1/messages 端点传入 gemini 模型名的场景（含前缀映射）
// 仅在 Antigravity 模式下运行（ENDPOINT_PREFIX="/antigravity"）
func TestClaudeMessagesWithGeminiModel(t *testing.T) {
	if endpointPrefix != "/antigravity" {
		t.Skip("仅在 Antigravity 模式下运行")
	}
	claudeKey := requireClaudeAPIKey(t)

	// 测试通过 Claude 端点调用 Gemini 模型
	geminiViaClaude := []string{
		"gemini-3-flash",       // 直接支持
		"gemini-3-pro-low",     // 直接支持
		"gemini-3-pro-high",    // 直接支持
		"gemini-3-pro",         // 前缀映射 -> gemini-3-pro-high
		"gemini-3-pro-preview", // 前缀映射 -> gemini-3-pro-high
	}

	for i, model := range geminiViaClaude {
		if i > 0 {
			time.Sleep(testInterval)
		}
		t.Run(model+"_通过Claude端点", func(t *testing.T) {
			testClaudeMessage(t, claudeKey, model, false)
		})
		time.Sleep(testInterval)
		t.Run(model+"_通过Claude端点_流式", func(t *testing.T) {
			testClaudeMessage(t, claudeKey, model, true)
		})
	}
}

// TestClaudeMessagesWithNoSignature 测试历史 thinking block 不带 signature 的场景
// 验证：Gemini 模型接受没有 signature 的 thinking block
func TestClaudeMessagesWithNoSignature(t *testing.T) {
	claudeKey := requireClaudeAPIKey(t)
	models := []string{
		"claude-haiku-4-5-20251001", // gemini-3-flash - 支持无 signature
	}
	for i, model := range models {
		if i > 0 {
			time.Sleep(testInterval)
		}
		t.Run(model+"_无signature", func(t *testing.T) {
			testClaudeWithNoSignature(t, claudeKey, model)
		})
	}
}

func testClaudeWithNoSignature(t *testing.T, claudeKey string, model string) {
	url := baseURL + endpointPrefix + "/v1/messages"

	// 模拟历史对话包含 thinking block 但没有 signature
	payload := map[string]any{
		"model":      model,
		"max_tokens": 200,
		"stream":     false,
		// 开启 thinking 模式
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 1024,
		},
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "What is 2+2?",
			},
			// assistant 消息包含 thinking block 但没有 signature
			map[string]any{
				"role": "assistant",
				"content": []map[string]any{
					{
						"type":     "thinking",
						"thinking": "Let me calculate 2+2...",
						// 故意不包含 signature
					},
					{
						"type": "text",
						"text": "2+2 equals 4.",
					},
				},
			},
			map[string]any{
				"role":    "user",
				"content": "What is 3+3?",
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+claudeKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 400 {
		t.Fatalf("无 signature thinking 处理失败，收到 400 错误: %s", string(respBody))
	}

	if resp.StatusCode == 503 {
		t.Skipf("账号暂时不可用 (503): %s", string(respBody))
	}

	if resp.StatusCode == 429 {
		t.Skipf("请求被限流 (429): %s", string(respBody))
	}

	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if result["type"] != "message" {
		t.Errorf("期望 type=message, 得到 %v", result["type"])
	}
	t.Logf("✅ 无 signature thinking 处理测试通过, id=%v", result["id"])
}

// TestGeminiEndpointWithClaudeModel 测试通过 Gemini 端点调用 Claude 模型
// 仅在 Antigravity 模式下运行（ENDPOINT_PREFIX="/antigravity"）
func TestGeminiEndpointWithClaudeModel(t *testing.T) {
	if endpointPrefix != "/antigravity" {
		t.Skip("仅在 Antigravity 模式下运行")
	}
	geminiKey := requireGeminiAPIKey(t)

	// 测试通过 Gemini 端点调用 Claude 模型
	claudeViaGemini := []string{
		"claude-sonnet-4-5",
		"claude-opus-4-5-thinking",
	}

	for i, model := range claudeViaGemini {
		if i > 0 {
			time.Sleep(testInterval)
		}
		t.Run(model+"_通过Gemini端点", func(t *testing.T) {
			testGeminiGenerate(t, geminiKey, model, false)
		})
		time.Sleep(testInterval)
		t.Run(model+"_通过Gemini端点_流式", func(t *testing.T) {
			testGeminiGenerate(t, geminiKey, model, true)
		})
	}
}
