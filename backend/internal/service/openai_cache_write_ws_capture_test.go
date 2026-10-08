package service

import (
	"reflect"
	"sync"
	"testing"
)

func TestOpenAICacheWriteWSPromptCapture_StatefulFramesDisableConnection(t *testing.T) {
	t.Parallel()
	request := []byte(`{"type":"response.create","model":"test-model","input":"explicit history"}`)
	for name, frame := range map[string]string{
		"session update":        `{"type":"session.update","session":{"instructions":"inherited secret"}}`,
		"conversation create":   `{"type":"conversation.item.create","item":{"role":"user","content":"inherited secret"}}`,
		"conversation delete":   `{"type":"conversation.item.delete","item_id":"item_1"}`,
		"conversation truncate": `{"type":"conversation.item.truncate","item_id":"item_1"}`,
		"audio buffer":          `{"type":"input_audio_buffer.commit"}`,
		"unknown":               `{"type":"future.state.update"}`,
		"opaque":                `not-json`,
	} {
		t.Run(name, func(t *testing.T) {
			var capture openAIWSCacheWritePromptCapture
			capture.observeOutbound(request)
			if !capture.snapshot().Valid {
				t.Fatal("fresh explicit response.create should have prompt evidence")
			}
			capture.observeOutbound([]byte(frame))
			if got := capture.snapshot(); got.Valid || got.ConfigHash != "" || len(got.InputHashes) != 0 {
				t.Fatal("stateful frame must immediately discard current prompt evidence")
			}
			capture.observeOutbound(request)
			capture.observeOutbound(request)
			if capture.snapshot().Valid {
				t.Fatal("later response.create must not restore trust on the same connection")
			}
			var fresh openAIWSCacheWritePromptCapture
			fresh.observeOutbound(request)
			if !fresh.snapshot().Valid {
				t.Fatal("stateful-frame exclusion should be connection-local")
			}
		})
	}
}

func TestOpenAICacheWriteWSPromptCapture_CancelPreservesExplicitEvidence(t *testing.T) {
	t.Parallel()
	var capture openAIWSCacheWritePromptCapture
	request := []byte(`{"type":"response.create","model":"test-model","input":"explicit history"}`)
	capture.observeOutbound(request)
	want := capture.snapshot()
	capture.observeOutbound([]byte(`{"type":"response.cancel"}`))
	if got := capture.snapshot(); !got.Valid || !reflect.DeepEqual(got, want) {
		t.Fatal("cancel does not change the explicit prompt; terminal validation handles the failure")
	}
	// Output evidence remains a statement about output alone. It must not
	// depend on whether the connection still has usable prompt evidence.
	capture.observeOutbound([]byte(`{"type":"session.update"}`))
	output := buildOpenAICacheWriteOutputEvidence([]byte(`{"status":"completed","output":[{"role":"assistant","content":"answer"}]}`))
	if capture.snapshot().Valid || !output.Valid {
		t.Fatal("prompt-state exclusion must not change output-only evidence semantics")
	}
}

func TestOpenAICacheWriteWSPromptCapture_ConcurrentSettlement(t *testing.T) {
	t.Parallel()
	var capture openAIWSCacheWritePromptCapture
	request := []byte(`{"type":"response.create","model":"test-model","input":"explicit history"}`)
	capture.observeOutbound(request)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < 100; i++ {
			capture.observeOutbound(request)
		}
		capture.observeOutbound([]byte(`{"type":"conversation.item.create"}`))
		for i := 0; i < 100; i++ {
			capture.observeOutbound(request)
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 1000; i++ {
			_ = capture.snapshot()
		}
	}()
	workers.Wait()
	if capture.snapshot().Valid {
		t.Fatal("concurrent settlement must not restore disabled prompt evidence")
	}
}
