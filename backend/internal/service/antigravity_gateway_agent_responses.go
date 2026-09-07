package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func agentSearchSources(raw json.RawMessage) []apicompat.ResponsesWebSearchSource {
	var grounding antigravity.GeminiGroundingMetadata
	if json.Unmarshal(raw, &grounding) != nil {
		return nil
	}
	var sources []apicompat.ResponsesWebSearchSource
	seen := map[string]bool{}
	for _, chunk := range grounding.GroundingChunks {
		if chunk.Web == nil {
			continue
		}
		parsed, err := url.Parse(chunk.Web.URI)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || seen[chunk.Web.URI] {
			continue
		}
		seen[chunk.Web.URI] = true
		sources = append(sources, apicompat.ResponsesWebSearchSource{Type: "url", URL: chunk.Web.URI, Title: chunk.Web.Title})
	}
	return sources
}

func addAgentSearchOutputs(response *apicompat.ResponsesResponse, searches []antigravityAgentSearchRecord) {
	var outputs []apicompat.ResponsesOutput
	var sourceText strings.Builder
	var annotations []apicompat.ResponsesAnnotation
	seen := map[string]bool{}
	for _, search := range searches {
		sources := agentSearchSources(search.turn.grounding)
		outputs = append(outputs, apicompat.ResponsesOutput{
			ID: "ws_" + uuid.NewString(), Type: "web_search_call", Status: "completed",
			Action: &apicompat.WebSearchAction{Type: "search", Query: search.query, Sources: sources},
		})
		for _, source := range sources {
			if seen[source.URL] {
				continue
			}
			seen[source.URL] = true
			if sourceText.Len() == 0 {
				_, _ = sourceText.WriteString("Search sources:\n")
			}
			label := strings.TrimSpace(source.Title)
			if label == "" {
				label = source.URL
			}
			start := utf8.RuneCountInString(sourceText.String())
			_, _ = sourceText.WriteString(label)
			annotations = append(annotations, apicompat.ResponsesAnnotation{Type: "url_citation", URL: source.URL, Title: label, StartIndex: start, EndIndex: start + utf8.RuneCountInString(label)})
			_ = sourceText.WriteByte('\n')
		}
	}
	// Grounding supports refer to the search response, not the model's later
	// answer. Label the actual source names rather than inventing claim offsets.
	if sourceText.Len() > 0 {
		part := apicompat.ResponsesContentPart{Type: "output_text", Text: sourceText.String(), Annotations: annotations}
		messageIndex := -1
		for i := range response.Output {
			if response.Output[i].Type == "message" {
				messageIndex = i
			}
		}
		if messageIndex >= 0 {
			response.Output[messageIndex].Content = append(response.Output[messageIndex].Content, part)
		} else {
			response.Output = append(response.Output, apicompat.ResponsesOutput{ID: "msg_" + uuid.NewString(), Type: "message", Role: "assistant", Status: "completed", Content: []apicompat.ResponsesContentPart{part}})
		}
	}
	response.Output = append(outputs, response.Output...)
}

func (s *AntigravityGatewayService) consumeAntigravityAgentResponses(c *gin.Context, call *antigravityCompatUpstreamCall, resp *http.Response) (*antigravityStreamResult, error) {
	claudeJSON, result, err := s.collectClaudeStreamResponse(c, resp, call.request.startTime, call.request.originalModel)
	if err != nil {
		return nil, s.mapAntigravityCompatCollectionError(c, err)
	}
	var claude apicompat.AnthropicResponse
	if err := json.Unmarshal(claudeJSON, &claude); err != nil {
		return nil, s.mapAntigravityCompatCollectionError(c, err)
	}
	response := apicompat.AnthropicToResponsesResponse(&claude)
	usage := call.agentResult.usage
	result.usage = &ClaudeUsage{InputTokens: max(0, usage.PromptTokenCount-usage.CachedContentTokenCount), OutputTokens: usage.CandidatesTokenCount + usage.ThoughtsTokenCount, CacheReadInputTokens: usage.CachedContentTokenCount}
	response.Usage = &apicompat.ResponsesUsage{
		InputTokens: usage.PromptTokenCount, OutputTokens: result.usage.OutputTokens,
		TotalTokens:         usage.PromptTokenCount + result.usage.OutputTokens,
		InputTokensDetails:  &apicompat.ResponsesInputTokensDetails{CachedTokens: usage.CachedContentTokenCount},
		OutputTokensDetails: &apicompat.ResponsesOutputTokensDetails{ReasoningTokens: usage.ThoughtsTokenCount},
	}
	addAgentSearchOutputs(response, call.agentResult.searches)
	if !call.request.clientStream {
		c.JSON(http.StatusOK, response)
		return result, nil
	}
	data, err := agentResponsesSSE(response)
	if err != nil {
		return nil, s.mapAntigravityCompatCollectionError(c, err)
	}
	if err := c.Request.Context().Err(); err != nil {
		return nil, err
	}
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Data(http.StatusOK, "text/event-stream", data)
	c.Writer.Flush()
	return result, nil
}

// Internal rounds never commit headers or data. Render one lifecycle after all
// upstream work succeeds, retaining safe retry/failover before that point.
func agentResponsesSSE(response *apicompat.ResponsesResponse) ([]byte, error) {
	var out bytes.Buffer
	sequence := 0
	emit := func(event apicompat.ResponsesStreamEvent) error {
		event.SequenceNumber = sequence
		sequence++
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		// The shared event type omits a zero sequence on lifecycle events.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &fields); err != nil {
			return err
		}
		fields["sequence_number"] = agentJSON(event.SequenceNumber)
		payload, err = json.Marshal(fields)
		if err != nil {
			return err
		}
		if out.Len()+len(payload) > antigravityAgentMaxHistoryBytes {
			return fmt.Errorf("responses stream exceeds byte limit")
		}
		fmt.Fprintf(&out, "event: %s\ndata: %s\n\n", event.Type, payload)
		return nil
	}
	initial := *response
	initial.Status, initial.Output, initial.Usage = "in_progress", []apicompat.ResponsesOutput{}, nil
	for _, kind := range []string{"response.created", "response.in_progress"} {
		if err := emit(apicompat.ResponsesStreamEvent{Type: kind, Response: &initial}); err != nil {
			return nil, err
		}
	}
	for index, item := range response.Output {
		added := item
		added.Status = "in_progress"
		added.Content, added.Summary, added.Arguments = nil, nil, ""
		if err := emit(apicompat.ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: index, Item: &added}); err != nil {
			return nil, err
		}
		base := apicompat.ResponsesStreamEvent{OutputIndex: index, ItemID: item.ID}
		switch item.Type {
		case "web_search_call":
			for _, kind := range []string{"response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed"} {
				base.Type = kind
				if err := emit(base); err != nil {
					return nil, err
				}
			}
		case "message":
			for contentIndex, part := range item.Content {
				base.ContentIndex = contentIndex
				empty := apicompat.ResponsesContentPart{Type: part.Type}
				base.Type, base.Part = "response.content_part.added", &empty
				if err := emit(base); err != nil {
					return nil, err
				}
				base.Part = nil
				base.Type, base.Delta = "response.output_text.delta", part.Text
				if err := emit(base); err != nil {
					return nil, err
				}
				base.Delta = ""
				for annotationIndex, annotation := range part.Annotations {
					base.Type, base.AnnotationIndex, base.Annotation = "response.output_text.annotation.added", annotationIndex, &annotation
					if err := emit(base); err != nil {
						return nil, err
					}
				}
				base.Annotation, base.AnnotationIndex = nil, 0
				base.Type, base.Text, base.Delta = "response.output_text.done", part.Text, ""
				if err := emit(base); err != nil {
					return nil, err
				}
				base.Type, base.Part, base.Text = "response.content_part.done", &part, ""
				if err := emit(base); err != nil {
					return nil, err
				}
				base.Part = nil
			}
		case "function_call":
			base.CallID, base.Name = item.CallID, item.Name
			base.Type, base.Delta = "response.function_call_arguments.delta", item.Arguments
			if err := emit(base); err != nil {
				return nil, err
			}
			base.Type, base.Arguments, base.Delta = "response.function_call_arguments.done", item.Arguments, ""
			if err := emit(base); err != nil {
				return nil, err
			}
		case "reasoning":
			for summaryIndex, summary := range item.Summary {
				base.SummaryIndex = summaryIndex
				part := apicompat.ResponsesContentPart{Type: "summary_text"}
				base.Type, base.Part = "response.reasoning_summary_part.added", &part
				if err := emit(base); err != nil {
					return nil, err
				}
				base.Part = nil
				base.Type, base.Delta = "response.reasoning_summary_text.delta", summary.Text
				if err := emit(base); err != nil {
					return nil, err
				}
				base.Type, base.Text, base.Delta = "response.reasoning_summary_text.done", summary.Text, ""
				if err := emit(base); err != nil {
					return nil, err
				}
				part.Text = summary.Text
				base.Type, base.Part, base.Text = "response.reasoning_summary_part.done", &part, ""
				if err := emit(base); err != nil {
					return nil, err
				}
				base.Part = nil
			}
		}
		if err := emit(apicompat.ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: index, Item: &item}); err != nil {
			return nil, err
		}
	}
	kind := "response.completed"
	if response.Status == "incomplete" {
		kind = "response.incomplete"
	}
	if err := emit(apicompat.ResponsesStreamEvent{Type: kind, Response: response}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
