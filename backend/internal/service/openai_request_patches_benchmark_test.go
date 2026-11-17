package service

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

var benchmarkOpenAIRequestPatchesSink []byte

// Compare the same patch list and immutable input, without timing request-view
// construction. BenchmarkOpenAIResponses_LargeInputRawPatch covers that too.
func BenchmarkOpenAIRequestPatches(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{
		{name: "small", bytes: 256},
		{name: "1MiB", bytes: 1 << 20},
		{name: "8MiB", bytes: 8 << 20},
	} {
		body := []byte(`{"model":"old","max_tokens":8192,"input":[{"type":"message","role":"user","content":"` + strings.Repeat("a", size.bytes) + `"}],"reasoning":{"effort":"low"},"previous_response_id":"resp_1","service_tier":"flex"}`)
		for _, edits := range []struct {
			name    string
			patches []openAIRequestPatch
		}{
			{
				name: "1_edit",
				patches: []openAIRequestPatch{
					{path: "model", value: "mapped"},
				},
			},
			{
				name: "6_edits",
				patches: []openAIRequestPatch{
					{path: "model", value: "mapped"},
					{path: "reasoning.effort", value: "none"},
					{path: "previous_response_id", delete: true},
					{path: "max_output_tokens", value: 8192},
					{path: "max_tokens", delete: true},
					{path: "service_tier", value: "priority"},
				},
			},
		} {
			view := requestViewWithPatches(body, edits.patches)
			want, err := applyOpenAIRequestPatchesSequential(body, edits.patches)
			if err != nil {
				b.Fatal(err)
			}
			got, err := view.ApplyPatches(context.Background())
			if err != nil || !bytes.Equal(want, got) {
				b.Fatalf("patch results differ: %v", err)
			}
			for _, implementation := range []struct {
				name  string
				apply func() ([]byte, error)
			}{
				{name: "SequentialSJSON", apply: func() ([]byte, error) { return applyOpenAIRequestPatchesSequential(body, edits.patches) }},
				{name: "ApplyPatches", apply: func() ([]byte, error) { return view.ApplyPatches(context.Background()) }},
			} {
				b.Run(size.name+"/"+edits.name+"/"+implementation.name, func(b *testing.B) {
					b.SetBytes(int64(len(body)))
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						patched, err := implementation.apply()
						if err != nil {
							b.Fatal(err)
						}
						benchmarkOpenAIRequestPatchesSink = patched
					}
				})
			}
		}
	}
}

// A small byte length can still contain thousands of object members. Keep this
// case beside the large opaque-input benchmarks to expose scratch amplification.
func BenchmarkOpenAIRequestPatches_DenseObjects(b *testing.B) {
	for _, fixture := range []struct {
		name   string
		nested bool
	}{
		{name: "root"},
		{name: "nested", nested: true},
	} {
		body, patches := denseOpenAIRequestPatchFixture(4096, fixture.nested)
		view := requestViewWithPatches(body, patches)
		if view.canBatchPatches() {
			b.Fatal("dense object should retain the sequential implementation")
		}
		want, err := applyOpenAIRequestPatchesSequential(body, patches)
		if err != nil {
			b.Fatal(err)
		}
		got, err := view.ApplyPatches(context.Background())
		if err != nil || !bytes.Equal(want, got) {
			b.Fatalf("patch results differ: %v", err)
		}
		for _, implementation := range []struct {
			name  string
			apply func() ([]byte, error)
		}{
			{name: "SequentialSJSON", apply: func() ([]byte, error) { return applyOpenAIRequestPatchesSequential(body, patches) }},
			{name: "ApplyPatches", apply: func() ([]byte, error) { return view.ApplyPatches(context.Background()) }},
		} {
			b.Run(fixture.name+"/"+implementation.name, func(b *testing.B) {
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					patched, err := implementation.apply()
					if err != nil {
						b.Fatal(err)
					}
					benchmarkOpenAIRequestPatchesSink = patched
				}
			})
		}
	}
}

// Rechecking an edited parent through Doc.Get can materialize its large opaque
// sibling after every edit. The batched path should allocate one final body.
func BenchmarkOpenAIRequestPatches_RepeatedNestedLeaf(b *testing.B) {
	body, patches := repeatedNestedOpenAIRequestPatchFixture()
	view := requestViewWithPatches(body, patches)
	for _, implementation := range []struct {
		name  string
		apply func() ([]byte, error)
	}{
		{name: "SequentialSJSON", apply: func() ([]byte, error) { return applyOpenAIRequestPatchesSequential(body, patches) }},
		{name: "ApplyPatches", apply: func() ([]byte, error) { return view.ApplyPatches(context.Background()) }},
	} {
		b.Run(implementation.name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				patched, err := implementation.apply()
				if err != nil {
					b.Fatal(err)
				}
				benchmarkOpenAIRequestPatchesSink = patched
			}
		})
	}
}
