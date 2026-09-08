package backend

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"testing"
)

func TestAgnesAdapterMapsOpenAIImageRequest(t *testing.T) {
	outgoing, err := buildAgnesImageRequest(RouteConfig{
		Platform: "agnes",
		BaseURL:  "https://apihub.agnes-ai.com/v1",
	}, "agnes-image-2.1-flash", "golden hour city", "1536x1024", "b64_json", nil)
	if err != nil {
		t.Fatalf("BuildRequest() error = %v", err)
	}
	if outgoing.Model != "agnes-image-2.1-flash" || outgoing.Size != "1K" || outgoing.Ratio != "3:2" || !outgoing.ReturnBase64 {
		t.Fatalf("outgoing request = %#v", outgoing)
	}
}

func TestAgnesAdapterNormalizesResponse(t *testing.T) {
	response, err := NewAgnesAdapter().ParseResponse([]byte(`{"created":1,"data":[{"url":"https://cdn.example/image.png","b64_json":null}]}`))
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if response.Created != 1 || len(response.Data) != 1 || response.Data[0].URL != "https://cdn.example/image.png" {
		t.Fatalf("response = %#v", response)
	}
}

func TestAgnesAdapterDecodesGzipResponse(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`{"created":1,"data":[{"url":"https://cdn.example/image.png"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	body, err := decodeAgnesResponseBody(http.Header{"Content-Encoding": []string{"gzip"}}, compressed.Bytes())
	if err != nil {
		t.Fatalf("decodeAgnesResponseBody() error = %v", err)
	}
	response, err := NewAgnesAdapter().ParseResponse(body)
	if err != nil || response.Data[0].URL != "https://cdn.example/image.png" {
		t.Fatalf("response = %#v, error = %v", response, err)
	}
}

func TestDefaultPlatformRegistryListsRegisteredPlatforms(t *testing.T) {
	platforms := NewDefaultPlatformRegistry().Platforms()
	if len(platforms) != 3 {
		t.Fatalf("platform count = %d, want 3", len(platforms))
	}
	want := []PlatformDescriptor{
		{Key: "agnes", DisplayName: "Agnes", Operation: "images/generations", Protocol: "agnes-image", DefaultBaseURL: "https://apihub.agnes-ai.com/v1"},
		{Key: "openai", DisplayName: "OpenAI", Protocol: "transparent", DefaultBaseURL: "https://api.openai.com/v1"},
		{Key: "opencode", DisplayName: "OpenCode", Protocol: "opencode", DefaultBaseURL: "https://opencode.ai/zen"},
	}
	for index, platform := range platforms {
		if platform != want[index] {
			t.Fatalf("platform[%d] = %#v, want %#v", index, platform, want[index])
		}
	}
}
