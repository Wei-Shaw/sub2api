package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func feedGuardChunks(chunks ...string) (string, bool) {
	var g turnBoundaryGuard
	var out strings.Builder
	for _, c := range chunks {
		safe, triggered := g.Feed(c)
		out.WriteString(safe)
		if triggered {
			return out.String(), true
		}
	}
	out.WriteString(g.FlushAndResetBlock())
	return out.String(), false
}

func TestTurnBoundaryGuard_DetectsLeakedTurnsAcrossAllChunkSplits(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantSafe string
	}{
		{
			name:     "claude code system reminder after prose",
			input:    "Done editing.\nuser\n<system-reminder>\nprompt.submit hook\n</system-reminder>\nNow run tests.\nmodel\n",
			wantSafe: "Done editing.\n",
		},
		{
			name:     "crlf user and system reminder",
			input:    "All set.\r\nuser\r\n<system-reminder>\r\nhook\r\n",
			wantSafe: "All set.\r\n",
		},
		{
			name:     "at very start of text",
			input:    "user\n<system-reminder>\nhook\n</system-reminder>\nfake prompt",
			wantSafe: "",
		},
		{
			name:     "after closed markdown code fence",
			input:    "Summary:\n```txt\nuser\n<system-reminder>\nquoted example\n```\nShould I apply this?\nuser\n<system-reminder>\nYes apply it.\nmodel\n",
			wantSafe: "Summary:\n```txt\nuser\n<system-reminder>\nquoted example\n```\nShould I apply this?\n",
		},
		{
			name:     "after inline triple backtick span on one line",
			input:    "```foo``` is inline.\nuser\n<system-reminder>\nhallucinated turn",
			wantSafe: "```foo``` is inline.\n",
		},
		{
			name:     "double user line before marker",
			input:    "Roles:\nuser\nuser\n<system-reminder>\nhallucinated turn",
			wantSafe: "Roles:\nuser\n",
		},
		{
			name:     "codex environment context",
			input:    "Updated file.\nuser\n<environment_context>\n<cwd>/tmp</cwd>\n",
			wantSafe: "Updated file.\n",
		},
		{
			name:     "task notification",
			input:    "Waiting.\nuser\n<task-notification>\n<status>completed</status>\n",
			wantSafe: "Waiting.\n",
		},
		{
			name:     "raw start_of_turn user",
			input:    "Finished.\n<start_of_turn>user\nNext question",
			wantSafe: "Finished.\n",
		},
		{
			name:     "raw end_of_turn control token",
			input:    "Finished.\n<end_of_turn>\nuser\nNext question",
			wantSafe: "Finished.\n",
		},
		{
			name:     "raw pipe end_of_turn token",
			input:    "Finished.\n<|end_of_turn|>\nNext question",
			wantSafe: "Finished.\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Whole chunk
			got, triggered := feedGuardChunks(tc.input)
			require.True(t, triggered)
			require.Equal(t, tc.wantSafe, got)

			// 1-byte chunks
			byteChunks := make([]string, len(tc.input))
			for i := range tc.input {
				byteChunks[i] = tc.input[i : i+1]
			}
			got, triggered = feedGuardChunks(byteChunks...)
			require.True(t, triggered)
			require.Equal(t, tc.wantSafe, got)

			// Every possible 2-way split
			for split := 1; split < len(tc.input); split++ {
				got, triggered = feedGuardChunks(tc.input[:split], tc.input[split:])
				require.True(t, triggered, "split=%d", split)
				require.Equal(t, tc.wantSafe, got, "split=%d", split)
			}
		})
	}
}

func TestTurnBoundaryGuard_NoFalsePositives(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name:  "quoted leak inside backtick code fence",
			input: "Here is what leaked:\n```\nuser\n<system-reminder>\nprompt.submit hook\n</system-reminder>\nfake prompt\nmodel\n```\nEnd of quote.\n",
		},
		{
			name:  "quoted leak inside 4-backtick fence containing 3-backticks",
			input: "````markdown\n```\nuser\n<system-reminder>\nstill inside\n```\nuser\n<system-reminder>\nstill inside\n````\nDone.",
		},
		{
			name:  "quoted leak inside tilde fence",
			input: "~~~text\nuser\n<system-reminder>\ninside tilde\n~~~\nDone.",
		},
		{
			name:  "user line followed by code fence containing marker",
			input: "Roles:\nuser\n```\nuser\n<system-reminder>\nquoted\n```\nDone.",
		},
		{
			name:  "standalone user line followed by normal text",
			input: "Roles:\nuser\nassistant\nmodel\n",
		},
		{
			name:  "mid-line user and system-reminder",
			input: "Mentioning user <system-reminder> inline should never stop.\n",
		},
		{
			name:  "trailing user prefix at end of stream",
			input: "Selected role:\nuser",
		},
		{
			name:  "trailing user newline at end of stream",
			input: "Selected role:\nuser\n",
		},
		{
			name:  "inline control token mention",
			input: "The token <|end_of_turn|> marks the end of a turn.\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, triggered := feedGuardChunks(tc.input)
			require.False(t, triggered)
			require.Equal(t, tc.input, got)

			// Byte-by-byte streaming must match identically.
			byteChunks := make([]string, len(tc.input))
			for i := range tc.input {
				byteChunks[i] = tc.input[i : i+1]
			}
			got, triggered = feedGuardChunks(byteChunks...)
			require.False(t, triggered)
			require.Equal(t, tc.input, got)
		})
	}
}

type parsedAnthropicSSE struct {
	text       string
	thinking   string
	toolNames  []string
	stopReason string
	stopCount  int
}

func parseAnthropicSSE(t *testing.T, raw []byte) parsedAnthropicSSE {
	t.Helper()
	var res parsedAnthropicSSE
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var ev map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &ev))
		switch ev["type"] {
		case "content_block_start":
			cb, _ := ev["content_block"].(map[string]any)
			if cb["type"] == "tool_use" {
				name, _ := cb["name"].(string)
				res.toolNames = append(res.toolNames, name)
			}
		case "content_block_delta":
			d, _ := ev["delta"].(map[string]any)
			switch d["type"] {
			case "text_delta":
				s, _ := d["text"].(string)
				res.text += s
			case "thinking_delta":
				s, _ := d["thinking"].(string)
				res.thinking += s
			}
		case "message_delta":
			d, _ := ev["delta"].(map[string]any)
			if sr, ok := d["stop_reason"].(string); ok {
				res.stopReason = sr
			}
		case "message_stop":
			res.stopCount++
		}
	}
	return res
}

func TestStreamingAndNonStreaming_TruncateLeakedTurnAndDropHallucinatedToolUse(t *testing.T) {
	// Reproduces the real incident where Gemini finishes a reply, simulates a
	// user turn starting with "user\n<system-reminder>", and then emits a tool call.
	chunks := []GeminiResponse{
		{
			ResponseID: "resp_leak_1",
			Candidates: []GeminiCandidate{{
				Content: &GeminiContent{
					Role: "model",
					Parts: []GeminiPart{
						{Thought: true, Text: "Checking draft before replying."},
						{Text: "Updated `.writing/rebuttal-draft.md`.\nuser\n"},
					},
				},
			}},
			UsageMetadata: &GeminiUsageMetadata{PromptTokenCount: 500, CandidatesTokenCount: 40},
		},
		{
			ResponseID: "resp_leak_1",
			Candidates: []GeminiCandidate{{
				Content: &GeminiContent{
					Role: "model",
					Parts: []GeminiPart{
						{Text: "<system-reminder>\nprompt.submit hook\n</system-reminder>\nRun git status.\nmodel\n"},
						{FunctionCall: &GeminiFunctionCall{ID: "call_bad", Name: "Bash", Args: map[string]any{"command": "git status"}}},
					},
				},
				FinishReason: "STOP",
			}},
			UsageMetadata: &GeminiUsageMetadata{PromptTokenCount: 500, CandidatesTokenCount: 120},
		},
	}

	// Streaming path
	sp := NewStreamingProcessor("gemini-4-neon")
	var rawSSE []byte
	for _, ch := range chunks {
		b, err := json.Marshal(map[string]any{"response": ch})
		require.NoError(t, err)
		rawSSE = append(rawSSE, sp.ProcessLine("data: "+string(b))...)
	}
	finalSSE, usage := sp.Finish()
	rawSSE = append(rawSSE, finalSSE...)

	require.True(t, sp.TruncatedOnTurnBoundary())
	parsed := parseAnthropicSSE(t, rawSSE)
	require.Equal(t, "Updated `.writing/rebuttal-draft.md`.\n", parsed.text)
	require.Equal(t, "Checking draft before replying.", parsed.thinking)
	require.Empty(t, parsed.toolNames, "hallucinated tool_use after leaked turn must be dropped")
	require.Equal(t, "end_turn", parsed.stopReason)
	require.Equal(t, 1, parsed.stopCount)
	require.Equal(t, 500, usage.InputTokens)
	require.Equal(t, 120, usage.OutputTokens)

	// Non-streaming parity
	var allParts []GeminiPart
	for _, ch := range chunks {
		allParts = append(allParts, ch.Candidates[0].Content.Parts...)
	}
	nonStreamBody, err := json.Marshal(V1InternalResponse{
		ResponseID: "resp_leak_1",
		Response: GeminiResponse{
			Candidates: []GeminiCandidate{{
				Content:      &GeminiContent{Role: "model", Parts: allParts},
				FinishReason: "STOP",
			}},
			UsageMetadata: &GeminiUsageMetadata{PromptTokenCount: 500, CandidatesTokenCount: 120},
		},
	})
	require.NoError(t, err)

	claudeBytes, _, err := TransformGeminiToClaude(nonStreamBody, "gemini-4-neon")
	require.NoError(t, err)
	var claudeResp ClaudeResponse
	require.NoError(t, json.Unmarshal(claudeBytes, &claudeResp))
	require.Equal(t, "end_turn", claudeResp.StopReason)

	var nsText, nsThinking string
	var nsTools []string
	for _, blk := range claudeResp.Content {
		switch blk.Type {
		case "text":
			nsText += blk.Text
		case "thinking":
			nsThinking += blk.Thinking
		case "tool_use":
			nsTools = append(nsTools, blk.Name)
		}
	}
	require.Equal(t, parsed.text, nsText)
	require.Equal(t, parsed.thinking, nsThinking)
	require.Empty(t, nsTools)
}

func TestStreamingAndNonStreaming_PreservesEarlierToolCallAndTrailingUserBeforeTool(t *testing.T) {
	t.Run("trailing user text before legitimate tool call is preserved", func(t *testing.T) {
		body, err := json.Marshal(V1InternalResponse{
			ResponseID: "resp_ok_tool",
			Response: GeminiResponse{
				Candidates: []GeminiCandidate{{
					Content: &GeminiContent{
						Role: "model",
						Parts: []GeminiPart{
							{Text: "Checking role:\nuser\n"},
							{FunctionCall: &GeminiFunctionCall{ID: "call_ok", Name: "Read", Args: map[string]any{"path": "a.txt"}}},
						},
					},
					FinishReason: "STOP",
				}},
			},
		})
		require.NoError(t, err)

		sp := NewStreamingProcessor("gemini-4-neon")
		rawSSE := sp.ProcessLine("data: " + string(body))
		finalSSE, _ := sp.Finish()
		rawSSE = append(rawSSE, finalSSE...)
		require.False(t, sp.TruncatedOnTurnBoundary())
		parsed := parseAnthropicSSE(t, rawSSE)
		require.Equal(t, "Checking role:\nuser\n", parsed.text)
		require.Equal(t, []string{"Read"}, parsed.toolNames)
		require.Equal(t, "tool_use", parsed.stopReason)

		claudeBytes, _, err := TransformGeminiToClaude(body, "gemini-4-neon")
		require.NoError(t, err)
		var claudeResp ClaudeResponse
		require.NoError(t, json.Unmarshal(claudeBytes, &claudeResp))
		require.Equal(t, "tool_use", claudeResp.StopReason)
		require.Len(t, claudeResp.Content, 2)
		require.Equal(t, "Checking role:\nuser\n", claudeResp.Content[0].Text)
		require.Equal(t, "Read", claudeResp.Content[1].Name)
	})

	t.Run("legitimate tool call before leaked turn is kept while later tool call is dropped", func(t *testing.T) {
		body, err := json.Marshal(V1InternalResponse{
			ResponseID: "resp_mixed_tool",
			Response: GeminiResponse{
				Candidates: []GeminiCandidate{{
					Content: &GeminiContent{
						Role: "model",
						Parts: []GeminiPart{
							{FunctionCall: &GeminiFunctionCall{ID: "call_1", Name: "Read", Args: map[string]any{"path": "ok.txt"}}},
							{Text: "Done.\nuser\n<system-reminder>\nhook\n</system-reminder>\nDelete it.\nmodel\n"},
							{FunctionCall: &GeminiFunctionCall{ID: "call_2", Name: "Bash", Args: map[string]any{"command": "rm -rf /"}}},
						},
					},
					FinishReason: "STOP",
				}},
			},
		})
		require.NoError(t, err)

		sp := NewStreamingProcessor("gemini-4-neon")
		rawSSE := sp.ProcessLine("data: " + string(body))
		finalSSE, _ := sp.Finish()
		rawSSE = append(rawSSE, finalSSE...)
		require.True(t, sp.TruncatedOnTurnBoundary())
		parsed := parseAnthropicSSE(t, rawSSE)
		require.Equal(t, "Done.\n", parsed.text)
		require.Equal(t, []string{"Read"}, parsed.toolNames)
		require.Equal(t, "tool_use", parsed.stopReason)
	})

	t.Run("thinking containing leaked marker is not truncated", func(t *testing.T) {
		body, err := json.Marshal(V1InternalResponse{
			ResponseID: "resp_think",
			Response: GeminiResponse{
				Candidates: []GeminiCandidate{{
					Content: &GeminiContent{
						Role: "model",
						Parts: []GeminiPart{
							{Thought: true, Text: "The user prompt had:\nuser\n<system-reminder>\nfoo"},
							{Text: "Here is the answer."},
						},
					},
					FinishReason: "STOP",
				}},
			},
		})
		require.NoError(t, err)

		sp := NewStreamingProcessor("gemini-4-neon")
		rawSSE := sp.ProcessLine("data: " + string(body))
		finalSSE, _ := sp.Finish()
		rawSSE = append(rawSSE, finalSSE...)
		require.False(t, sp.TruncatedOnTurnBoundary())
		parsed := parseAnthropicSSE(t, rawSSE)
		require.Equal(t, "Here is the answer.", parsed.text)
		require.Contains(t, parsed.thinking, "<system-reminder>")
	})
}
