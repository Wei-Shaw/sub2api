package cursor

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseAvailableModelsResponse_PrefersNestedModels(t *testing.T) {
	var opus ProtobufWriter
	opus.String(1, "claude-opus-5")
	opus.Bool(2, true)
	opus.String(17, "Claude Opus 5")
	opus.String(18, "claude-opus-5")
	opus.String(37, "opus")
	opus.String(37, "opus-latest")

	var hidden ProtobufWriter
	hidden.String(1, "secret-model")
	hidden.Bool(35, true)

	var namesOnly ProtobufWriter
	namesOnly.String(1, "should-ignore-when-models-present")

	var resp ProtobufWriter
	resp.String(1, "should-ignore-when-models-present")
	resp.Bytes(2, opus.Result())
	resp.Bytes(2, hidden.Result())
	resp.Bytes(2, func() []byte {
		var gpt ProtobufWriter
		gpt.String(1, "gpt-5.6-sol")
		gpt.String(17, "GPT-5.6 Sol")
		return gpt.Result()
	}())

	models := ParseAvailableModelsResponse(resp.Result())
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2 (hidden skipped): %+v", len(models), models)
	}
	if models[0].Name != "claude-opus-5" || !models[0].DefaultOn || models[0].DisplayName != "Claude Opus 5" {
		t.Fatalf("opus: %+v", models[0])
	}
	if got := models[0].Aliases; len(got) != 2 || got[0] != "opus" || got[1] != "opus-latest" {
		t.Fatalf("aliases: %v", got)
	}
	if models[1].Name != "gpt-5.6-sol" {
		t.Fatalf("gpt: %+v", models[1])
	}
	ids := ModelIDs(models)
	if len(ids) != 2 || ids[0] != "claude-opus-5" || ids[1] != "gpt-5.6-sol" {
		t.Fatalf("ids: %v", ids)
	}
}

func TestParseAvailableModelsResponse_FallsBackToModelNames(t *testing.T) {
	var resp ProtobufWriter
	resp.String(1, "default")
	resp.String(1, "composer-2.5")
	models := ParseAvailableModelsResponse(resp.Result())
	if len(models) != 2 || models[0].Name != "default" || models[1].Name != "composer-2.5" {
		t.Fatalf("got %+v", models)
	}
}

func TestParseAvailableModelsHTTPBody_GzipProtobuf(t *testing.T) {
	var model ProtobufWriter
	model.String(1, "grok-4.6")
	var resp ProtobufWriter
	resp.Bytes(2, model.Result())

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(resp.Result()); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	models, err := parseAvailableModelsHTTPBody(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Name != "grok-4.6" {
		t.Fatalf("got %+v", models)
	}
}

func TestParseAvailableModelsHTTPBody_ConnectEnvelope(t *testing.T) {
	var model ProtobufWriter
	model.String(1, "grok-4.6")
	var resp ProtobufWriter
	resp.Bytes(2, model.Result())
	frame, err := EncodeFrame(resp.Result(), false)
	if err != nil {
		t.Fatal(err)
	}
	models, err := parseAvailableModelsHTTPBody(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Name != "grok-4.6" {
		t.Fatalf("got %+v", models)
	}
}

func TestClientAvailableModels_ProtoUnary(t *testing.T) {
	var model ProtobufWriter
	model.String(1, "composer-2.5")
	model.String(17, "Composer 2.5")
	var resp ProtobufWriter
	resp.Bytes(2, model.Result())

	var sawProto bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != EndpointModels {
			t.Errorf("path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		switch r.Header.Get("content-type") {
		case "application/proto":
			sawProto = true
			if r.Header.Get("connect-protocol-version") != "" {
				t.Errorf("proto request should not send connect-protocol-version")
			}
			if !bytes.Equal(body, encodeAvailableModelsRequest()) {
				t.Errorf("request payload mismatch: %x", body)
			}
			w.Header().Set("Content-Type", "application/proto")
			_, _ = w.Write(resp.Result())
		default:
			http.Error(w, "unexpected", http.StatusUnsupportedMediaType)
		}
	}))
	defer server.Close()

	client := NewClient(Credentials{AccessToken: "tok", MachineID: "m"})
	client.HTTPClient = server.Client()
	client.APIBaseURL = server.URL

	models, err := client.AvailableModels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !sawProto {
		t.Fatal("expected application/proto request")
	}
	if len(models) != 1 || models[0].Name != "composer-2.5" || models[0].DisplayName != "Composer 2.5" {
		t.Fatalf("got %+v", models)
	}
}

func TestEncodeAvailableModelsRequest_MatchesIDEFlags(t *testing.T) {
	got := encodeAvailableModelsRequest()
	r := NewProtobufReader(got)
	flags := map[uint32]uint64{}
	for {
		f, err := r.Next()
		if f == nil || err != nil {
			break
		}
		flags[f.Num] = f.Varint
	}
	if flags[3] != 1 || flags[5] != 1 || flags[11] != 1 {
		t.Fatalf("flags %+v, want exclude_max_named_models/use_model_parameters/use_react_model_picker", flags)
	}
}
