package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestOpenAICacheWriteOutputCapture_RequiresCompleteTerminal(t *testing.T) {
	t.Parallel()
	item := `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"private-output-marker"}]}`
	terminal := `{"type":"response.completed","response":{"status":"completed","output":[` + item + `]}}`
	done := `{"type":"response.output_item.done","output_index":0,"item":` + item + `}`
	for _, test := range []struct {
		name   string
		events []string
		valid  bool
	}{
		{"complete", []string{terminal}, true},
		{"done agrees", []string{done, terminal}, true},
		{"progress agrees", []string{`{"type":"response.output_text.delta","output_index":0,"delta":"private-output-marker"}`, terminal}, true},
		{"explicit empty", []string{`{"type":"response.completed","response":{"output":[]}}`}, true},
		{"progress only", []string{done}, false},
		{"missing output", []string{`{"type":"response.completed","response":{"status":"completed"}}`}, false},
		{"omitted output", []string{done, `{"type":"response.completed","response":{"output":[]}}`}, false},
		{"unindexed tool delta", []string{`{"type":"response.function_call_arguments.delta","delta":"{}"}`, `{"type":"response.completed","response":{"output":[]}}`}, false},
		{"partial array", []string{`{"type":"response.output_text.delta","output_index":1,"delta":"more"}`, terminal}, false},
		{"partial text", []string{done, strings.Replace(terminal, "private-output-marker", "private", 1)}, false},
		{"failed", []string{`{"type":"response.failed","response":{"output":[` + item + `]}}`}, false},
		{"incomplete", []string{`{"type":"response.incomplete","response":{"output":[` + item + `]}}`}, false},
		{"failed then completed", []string{`{"type":"error","message":"failed"}`, terminal}, false},
		{"multiple terminals", []string{terminal, terminal}, false},
		{"output after terminal", []string{terminal, done}, false},
		{"new response after terminal", []string{terminal, `{"type":"response.created","response":{"output":[]}}`}, false},
		{"oversized index", []string{`{"type":"response.output_text.delta","output_index":256,"delta":"more"}`, terminal}, false},
		{"fractional index", []string{`{"type":"response.output_text.delta","output_index":0.5,"delta":"more"}`, terminal}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var capture openAICacheWriteOutputCapture
			for _, event := range test.events {
				capture.observe([]byte(event), "")
			}
			if capture.evidence.Valid != test.valid {
				t.Fatalf("valid = %v, want %v", capture.evidence.Valid, test.valid)
			}
			if strings.Contains(fmt.Sprintf("%+v", capture), "private-output-marker") {
				t.Fatal("capture retained plaintext output")
			}
		})
	}
}

func TestOpenAICacheWriteOutputCapture_EventHeaderAndRewrittenBody(t *testing.T) {
	t.Parallel()
	response := `{"response":{"status":"completed","output":[{"role":"assistant","content":"answer"}]}}`
	var capture openAICacheWriteOutputCapture
	capture.observe([]byte(response), "response.done")
	if !capture.evidence.Valid {
		t.Fatal("an SSE event header must identify a successful terminal")
	}
	resp := &http.Response{Body: &responsesClientToolStreamBody{}}
	rewritten := newOpenAICacheWriteOutputCapture(resp)
	rewritten.observe([]byte(response), "response.completed")
	if rewritten.evidence.Valid {
		t.Fatal("already-rewritten stream bodies must not establish upstream lineage")
	}
}

func TestOpenAICacheWriteOutputCapture_BufferedSSERejectsReconstruction(t *testing.T) {
	t.Parallel()
	resp := &http.Response{}
	body := "data: " + `{"type":"response.output_text.delta","output_index":0,"delta":"partial"}` + "\n\n" +
		"event: response.completed\ndata: " + `{"response":{"status":"completed","output":[]}}` + "\n\n"
	if captureOpenAICacheWriteSSEOutput(resp, body).Valid {
		t.Fatal("reconstructable deltas are not complete original terminal output")
	}
	body = "event: response.done\ndata: " + `{"response":{"status":"completed","output":[{"role":"assistant","content":"answer"}]}}` + "\n\n"
	if !captureOpenAICacheWriteSSEOutput(resp, body).Valid {
		t.Fatal("complete buffered SSE terminal should be captured")
	}
}

func TestOpenAICacheWritePromptCapture_UsesFinalRequestBytes(t *testing.T) {
	t.Parallel()
	original := `{"model":"test-model","input":"before rewrite"}`
	final := `{"model":"test-model","input":"after rewrite"}`
	req, err := http.NewRequest(http.MethodPost, "https://example.test/v1/responses", strings.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	req.Body = io.NopCloser(strings.NewReader(final))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(final)), nil }
	req.ContentLength = int64(len(final))
	want := buildOpenAICacheWritePromptEvidence([]byte(final))
	got := captureOpenAICacheWritePromptEvidence(req)
	if !got.Valid || !reflect.DeepEqual(got, want) {
		t.Fatal("evidence must match final outbound request")
	}
	remaining, err := io.ReadAll(req.Body)
	if err != nil || string(remaining) != final {
		t.Fatal("capturing evidence consumed or changed the outbound reader")
	}
	if openAICacheWritePromptsIdentical(got, buildOpenAICacheWritePromptEvidence([]byte(original))) {
		t.Fatal("pre-rewrite request must not be used")
	}
}

func TestOpenAICacheWritePromptCapture_FailsClosed(t *testing.T) {
	t.Parallel()
	for _, req := range []*http.Request{
		nil,
		{},
		{GetBody: func() (io.ReadCloser, error) { return nil, errors.New("unreadable") }},
		{GetBody: func() (io.ReadCloser, error) { return nil, nil }},
		{ContentLength: openAICacheWriteEvidenceMaxBodyBytes + 1, GetBody: func() (io.ReadCloser, error) {
			t.Fatal("oversized body should be skipped without reading")
			return nil, nil
		}},
		{GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(strings.Repeat(" ", openAICacheWriteEvidenceMaxBodyBytes+1))), nil
		}},
	} {
		if captureOpenAICacheWritePromptEvidence(req).Valid {
			t.Fatal("missing, unreadable or oversized request must not establish lineage")
		}
	}
}
