package backend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBatchClient_SubmitSingleImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/batches" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "plugin-image-history-1" {
			t.Fatalf("idempotency key = %q", got)
		}
		var request batchSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "gemini-2.5-flash-image-preview" || len(request.Items) != 1 {
			t.Fatalf("request = %#v", request)
		}
		item := request.Items[0]
		if item.CustomID != "plugin-image-history-1" || item.Prompt != "draw a cat" || item.OutputCount != 1 {
			t.Fatalf("item = %#v", item)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"imgbatch_1","status":"queued","model":"gemini-2.5-flash-image-preview"}`)
	}))
	defer server.Close()

	client := NewBatchClient(server.Client())
	job, err := client.Submit(context.Background(), server.URL, "api-key", "plugin-image-history-1", batchSubmitRequest{
		Model: "gemini-2.5-flash-image-preview",
		Items: []batchSubmitItem{{CustomID: "plugin-image-history-1", Prompt: "draw a cat", OutputCount: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "imgbatch_1" || job.Status != "queued" {
		t.Fatalf("job = %#v", job)
	}
}

func TestBatchClient_QueryContentAndCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer api-key" {
			t.Fatalf("authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_1":
			_, _ = io.WriteString(w, `{"id":"imgbatch_1","status":"completed","model":"model-1"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_1/items":
			_, _ = io.WriteString(w, `{"object":"list","data":[{"custom_id":"item/1","status":"completed","image_count":1}],"has_more":false}`)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/v1/images/batches/imgbatch_1/items/item%2F1/content":
			if got := r.URL.Query().Get("image_index"); got != "0" {
				t.Fatalf("image_index = %q", got)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-data"))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches/imgbatch_1/cancel":
			_, _ = io.WriteString(w, `{"id":"imgbatch_1","status":"cancelled"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewBatchClient(server.Client())
	job, err := client.Get(context.Background(), server.URL, "api-key", "imgbatch_1")
	if err != nil || job.Status != "completed" {
		t.Fatalf("Get() = %#v, %v", job, err)
	}
	items, err := client.ListItems(context.Background(), server.URL, "api-key", "imgbatch_1")
	if err != nil || len(items.Data) != 1 || items.Data[0].CustomID != "item/1" {
		t.Fatalf("ListItems() = %#v, %v", items, err)
	}
	content, mimeType, err := client.GetItemContent(context.Background(), server.URL, "api-key", "imgbatch_1", "item/1", 0)
	if err != nil || string(content) != "png-data" || mimeType != "image/png" {
		t.Fatalf("GetItemContent() = %q, %q, %v", content, mimeType, err)
	}
	job, err = client.Cancel(context.Background(), server.URL, "api-key", "imgbatch_1")
	if err != nil || job.Status != "cancelled" {
		t.Fatalf("Cancel() = %#v, %v", job, err)
	}
}
