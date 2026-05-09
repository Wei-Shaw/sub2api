package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConvertOpenCodeOpenAIModel_GPT54Capabilities(t *testing.T) {
	src := map[string]any{
		"id":          "gpt-5.4",
		"name":        "GPT-5.4",
		"attachment":  true,
		"reasoning":   true,
		"tool_call":   true,
		"interleaved": false,
		"modalities": map[string]any{
			"input":  []any{"text", "image", "pdf"},
			"output": []any{"text"},
		},
		"limit": map[string]any{
			"context": 1050000,
			"input":   922000,
			"output":  128000,
		},
		"cost": map[string]any{
			"input":      2.5,
			"output":     15.0,
			"cache_read": 0.25,
			"context_over_200k": map[string]any{
				"input":      5.0,
				"output":     22.5,
				"cache_read": 0.5,
			},
		},
		"release_date": "2026-03-05",
	}

	model, err := convertOpenCodeOpenAIModel(src)

	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", model.ID)
	require.Equal(t, "GPT-5.4", model.Name)
	require.True(t, model.Attachment)
	require.True(t, model.Reasoning)
	require.True(t, model.ToolCall)
	require.Equal(t, []string{"text", "image", "pdf"}, model.Modalities.Input)
	require.Equal(t, []string{"text"}, model.Modalities.Output)
	require.Equal(t, 1050000, model.Limit.Context)
	require.Equal(t, 922000, model.Limit.Input)
	require.Equal(t, 128000, model.Limit.Output)
	require.Equal(t, 5.0, model.Cost.ContextOver200K.Input)
	require.Equal(t, 22.5, model.Cost.ContextOver200K.Output)
	require.Equal(t, 0.5, model.Cost.ContextOver200K.CacheRead)
	require.Equal(t, "2026-03-05", model.ReleaseDate)
}

func TestExtractOpenCodeOpenAIModels_UsesBuiltInOpenAIProvider(t *testing.T) {
	payload := map[string]any{
		"openai": map[string]any{
			"models": map[string]any{
				"gpt-5.4": map[string]any{
					"id":   "gpt-5.4",
					"name": "GPT-5.4",
				},
				"gpt-5-chat-latest": map[string]any{
					"id":   "gpt-5-chat-latest",
					"name": "GPT-5 Chat Latest",
				},
				"gpt-5-alpha": map[string]any{
					"id":     "gpt-5-alpha",
					"name":   "GPT-5 Alpha",
					"status": "alpha",
				},
				"gpt-4.1-old": map[string]any{
					"id":     "gpt-4.1-old",
					"name":   "GPT-4.1 Old",
					"status": "deprecated",
				},
			},
		},
		"anthropic": map[string]any{
			"models": map[string]any{
				"claude-sonnet-4": map[string]any{
					"id":   "claude-sonnet-4",
					"name": "Claude Sonnet 4",
				},
			},
		},
	}

	models, err := extractOpenCodeOpenAIModels(payload)

	require.NoError(t, err)
	require.Contains(t, models, "gpt-5.4")
	require.NotContains(t, models, "gpt-5-chat-latest")
	require.NotContains(t, models, "gpt-5-alpha")
	require.NotContains(t, models, "gpt-4.1-old")
	require.NotContains(t, models, "claude-sonnet-4")
}

func TestExtractOpenCodeOpenAIModels_MaterializesFastModes(t *testing.T) {
	payload := map[string]any{
		"openai": map[string]any{
			"models": map[string]any{
				"gpt-5.4": map[string]any{
					"id":                "gpt-5.4",
					"name":              "GPT-5.4",
					"reasoning":         true,
					"attachment":        true,
					"tool_call":         true,
					"structured_output": true,
					"temperature":       false,
					"modalities": map[string]any{
						"input":  []any{"text", "image", "pdf"},
						"output": []any{"text"},
					},
					"cost":  map[string]any{"input": 2.5, "output": 15.0, "cache_read": 0.25},
					"limit": map[string]any{"context": 1050000, "input": 922000, "output": 128000},
					"experimental": map[string]any{
						"modes": map[string]any{
							"fast": map[string]any{
								"cost": map[string]any{"input": 5.0, "output": 30.0, "cache_read": 0.5},
								"provider": map[string]any{
									"body":    map[string]any{"service_tier": "priority"},
									"headers": map[string]any{"x-test-header": "fast-mode"},
								},
							},
						},
					},
				},
				"gpt-4o": map[string]any{
					"id":   "gpt-4o",
					"name": "GPT-4o",
					"experimental": map[string]any{
						"modes": map[string]any{
							"fast": map[string]any{
								"provider": map[string]any{
									"body":    map[string]any{"service_tier": "priority"},
									"headers": map[string]any{"x-test-header": "filtered-out"},
								},
							},
						},
					},
				},
			},
		},
	}

	models, err := extractOpenCodeOpenAIModels(payload)

	require.NoError(t, err)
	require.Contains(t, models, "gpt-5.4")
	require.Contains(t, models, "gpt-5.4-fast")
	require.NotContains(t, models, "gpt-5.4-fast-Sys")
	require.NotContains(t, models, "gpt-4o-fast")
	require.Equal(t, "gpt-5.4-fast", models["gpt-5.4-fast"].ID)
	require.Equal(t, "GPT-5.4 Fast", models["gpt-5.4-fast"].Name)
	require.Equal(t, 5.0, models["gpt-5.4-fast"].Cost.Input)
	require.Equal(t, 30.0, models["gpt-5.4-fast"].Cost.Output)
	require.Equal(t, "priority", requireOpenCodeModelOptions(t, models["gpt-5.4-fast"])["serviceTier"])
	require.Equal(t, "fast-mode", requireOpenCodeModelHeaders(t, models["gpt-5.4-fast"])["x-test-header"])
}

func TestExtractOpenCodeOpenAIModels_RestrictsGPT55CodexOAuthLimit(t *testing.T) {
	payload := map[string]any{
		"openai": map[string]any{
			"models": map[string]any{
				"gpt-5.5": map[string]any{
					"id":        "gpt-5.5",
					"name":      "GPT-5.5",
					"limit":     map[string]any{"context": 1050000, "input": 922000, "output": 128000},
					"cost":      map[string]any{"input": 3.0, "output": 18.0, "cache_read": 0.3},
					"reasoning": true,
					"experimental": map[string]any{
						"modes": map[string]any{
							"fast": map[string]any{
								"provider": map[string]any{
									"body": map[string]any{"service_tier": "priority"},
								},
							},
						},
					},
				},
				"gpt-5.4": map[string]any{
					"id":    "gpt-5.4",
					"name":  "GPT-5.4",
					"limit": map[string]any{"context": 1050000, "input": 922000, "output": 128000},
					"experimental": map[string]any{
						"modes": map[string]any{
							"fast": map[string]any{
								"provider": map[string]any{
									"body": map[string]any{"service_tier": "priority"},
								},
							},
						},
					},
				},
			},
		},
	}

	models, err := extractOpenCodeOpenAIModels(payload)

	require.NoError(t, err)
	require.Equal(t, 400000, models["gpt-5.5"].Limit.Context)
	require.Equal(t, 272000, models["gpt-5.5"].Limit.Input)
	require.Equal(t, 128000, models["gpt-5.5"].Limit.Output)
	require.Equal(t, 400000, models["gpt-5.5-fast"].Limit.Context)
	require.Equal(t, 272000, models["gpt-5.5-fast"].Limit.Input)
	require.Equal(t, 128000, models["gpt-5.5-fast"].Limit.Output)
	require.Equal(t, "priority", requireOpenCodeModelOptions(t, models["gpt-5.5-fast"])["serviceTier"])
	require.Equal(t, 1050000, models["gpt-5.4"].Limit.Context)
	require.Equal(t, 922000, models["gpt-5.4"].Limit.Input)
	require.Equal(t, 1050000, models["gpt-5.4-fast"].Limit.Context)
	require.Equal(t, 922000, models["gpt-5.4-fast"].Limit.Input)
}

func TestOpenCodeMetadataServiceGetOpenAIModels_UsesCacheOnFailure(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api.json", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openai": map[string]any{
				"models": map[string]any{
					"gpt-5.4": map[string]any{
						"id":         "gpt-5.4",
						"name":       "GPT-5.4",
						"attachment": true,
					},
				},
			},
		})
	}))
	defer ok.Close()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer bad.Close()

	svc := &OpenCodeMetadataService{
		client: ok.Client(),
		url:    ok.URL + "/api.json",
		ttl:    time.Minute,
	}

	models, err := svc.GetOpenAIModels(context.Background())
	require.NoError(t, err)
	require.Contains(t, models, "gpt-5.4")

	svc.url = bad.URL + "/api.json"
	models, err = svc.GetOpenAIModels(context.Background())
	require.NoError(t, err)
	require.Contains(t, models, "gpt-5.4")
	require.True(t, models["gpt-5.4"].Attachment)
}

func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_Success(t *testing.T) {
	npm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/omp-openai-provider-tools/latest", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "0.1.2"})
	}))
	defer npm.Close()

	svc := &OpenCodeMetadataService{
		client:       npm.Client(),
		npmLatestURL: npm.URL + "/omp-openai-provider-tools/latest",
		npmTTL:       time.Minute,
	}

	metadata := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "omp-openai-provider-tools", metadata.Package)
	require.Equal(t, "0.1.2", metadata.LatestVersion)
	require.Equal(t, "ok", metadata.Status)
	require.Empty(t, metadata.Error)
}

func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_RefreshesAfterTTL(t *testing.T) {
	versions := []string{"0.1.2", "9.9.9"}
	requestCount := 0
	npm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/omp-openai-provider-tools/latest", r.URL.Path)
		idx := requestCount
		if idx >= len(versions) {
			idx = len(versions) - 1
		}
		requestCount++
		_ = json.NewEncoder(w).Encode(map[string]any{"version": versions[idx]})
	}))
	defer npm.Close()

	svc := &OpenCodeMetadataService{
		client:       npm.Client(),
		npmLatestURL: npm.URL + "/omp-openai-provider-tools/latest",
		npmTTL:       time.Nanosecond,
	}

	first := svc.GetOMPProviderToolsMetadata(context.Background())
	time.Sleep(time.Millisecond)
	second := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "0.1.2", first.LatestVersion)
	require.Equal(t, "ok", first.Status)
	require.Equal(t, "9.9.9", second.LatestVersion)
	require.Equal(t, "ok", second.Status)
	require.Equal(t, 2, requestCount)
}

func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_UsesStaleCacheOnFailure(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/omp-openai-provider-tools/latest", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "0.1.2"})
	}))
	defer ok.Close()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/omp-openai-provider-tools/latest", r.URL.Path)
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer bad.Close()

	svc := &OpenCodeMetadataService{
		client:       ok.Client(),
		npmLatestURL: ok.URL + "/omp-openai-provider-tools/latest",
		npmTTL:       time.Nanosecond,
	}

	first := svc.GetOMPProviderToolsMetadata(context.Background())
	require.Equal(t, "0.1.2", first.LatestVersion)
	require.Equal(t, "ok", first.Status)
	require.Empty(t, first.Error)

	time.Sleep(time.Millisecond)
	svc.npmLatestURL = bad.URL + "/omp-openai-provider-tools/latest"
	stale := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "omp-openai-provider-tools", stale.Package)
	require.Equal(t, "0.1.2", stale.LatestVersion)
	require.Equal(t, "cached", stale.Status)
	require.NotEmpty(t, stale.Error)
}

func TestOpenCodeMetadataServiceGetOMPProviderToolsMetadata_UnavailableWithoutCache(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/omp-openai-provider-tools/latest", r.URL.Path)
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer bad.Close()

	svc := &OpenCodeMetadataService{
		client:       bad.Client(),
		npmLatestURL: bad.URL + "/omp-openai-provider-tools/latest",
		npmTTL:       time.Minute,
	}

	metadata := svc.GetOMPProviderToolsMetadata(context.Background())

	require.Equal(t, "omp-openai-provider-tools", metadata.Package)
	require.Empty(t, metadata.LatestVersion)
	require.Equal(t, "unavailable", metadata.Status)
	require.NotEmpty(t, metadata.Error)
}

func TestFilterOpenCodeOpenAIModelsForCodexOAuth(t *testing.T) {
	models := map[string]OpenCodeOpenAIModel{
		"gpt-4o":             {ID: "gpt-4o", Name: "GPT-4o"},
		"gpt-4o-fast":        {ID: "gpt-4o-fast", Name: "GPT-4o Fast"},
		"gpt-5.3":            {ID: "gpt-5.3", Name: "GPT-5.3"},
		"gpt-5.3-fast":       {ID: "gpt-5.3-fast", Name: "GPT-5.3 Fast"},
		"gpt-5.4":            {ID: "gpt-5.4", Name: "GPT-5.4"},
		"gpt-5.4-fast":       {ID: "gpt-5.4-fast", Name: "GPT-5.4 Fast"},
		"gpt-5.4-mini":       {ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
		"gpt-5.5":            {ID: "gpt-5.5", Name: "GPT-5.5"},
		"gpt-5.5-fast":       {ID: "gpt-5.5-fast", Name: "GPT-5.5 Fast"},
		"gpt-5.10":           {ID: "gpt-5.10", Name: "GPT-5.10"},
		"gpt-5.2":            {ID: "gpt-5.2", Name: "GPT-5.2"},
		"gpt-5.1-codex":      {ID: "gpt-5.1-codex", Name: "GPT-5.1 Codex"},
		"gpt-5.3-codex":      {ID: "gpt-5.3-codex", Name: "GPT-5.3 Codex"},
		"codex-mini-latest":  {ID: "codex-mini-latest", Name: "Codex Mini"},
		"gpt-5.2-codex":      {ID: "gpt-5.2-codex", Name: "GPT-5.2 Codex"},
		"gpt-5.1-codex-max":  {ID: "gpt-5.1-codex-max", Name: "GPT-5.1 Codex Max"},
		"gpt-5.1-codex-mini": {ID: "gpt-5.1-codex-mini", Name: "GPT-5.1 Codex Mini"},
	}

	filtered := filterOpenCodeOpenAIModelsForCodexOAuth(models)

	require.Contains(t, filtered, "gpt-5.4")
	require.Contains(t, filtered, "gpt-5.4-fast")
	require.Contains(t, filtered, "gpt-5.4-mini")
	require.Contains(t, filtered, "gpt-5.5")
	require.Contains(t, filtered, "gpt-5.5-fast")
	require.Contains(t, filtered, "gpt-5.10")
	require.Contains(t, filtered, "gpt-5.2")
	require.Contains(t, filtered, "gpt-5.1-codex")
	require.Contains(t, filtered, "gpt-5.3-codex")
	require.Contains(t, filtered, "codex-mini-latest")
	require.NotContains(t, filtered, "gpt-4o")
	require.NotContains(t, filtered, "gpt-4o-fast")
	require.NotContains(t, filtered, "gpt-5.3")
	require.NotContains(t, filtered, "gpt-5.3-fast")
}

func requireOpenCodeModelOptions(t *testing.T, model OpenCodeOpenAIModel) map[string]any {
	t.Helper()
	field := reflect.ValueOf(model).FieldByName("Options")
	require.True(t, field.IsValid(), "OpenCodeOpenAIModel.Options should exist")
	options, ok := field.Interface().(map[string]any)
	require.True(t, ok, "OpenCodeOpenAIModel.Options should be map[string]any")
	return options
}

func requireOpenCodeModelHeaders(t *testing.T, model OpenCodeOpenAIModel) map[string]string {
	t.Helper()
	field := reflect.ValueOf(model).FieldByName("Headers")
	require.True(t, field.IsValid(), "OpenCodeOpenAIModel.Headers should exist")
	headers, ok := field.Interface().(map[string]string)
	require.True(t, ok, "OpenCodeOpenAIModel.Headers should be map[string]string")
	return headers
}
