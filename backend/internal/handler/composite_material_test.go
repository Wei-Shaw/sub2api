package handler

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/rpc/innerpb"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"trpc.group/trpc-go/trpc-go/client"
)

type compositeMaterialClientStub struct {
	requests []*innerpb.UploadMaterialRequest
}

func (s *compositeMaterialClientStub) UploadMaterial(_ context.Context, req *innerpb.UploadMaterialRequest, _ ...client.Option) (*innerpb.UploadMaterialResponse, error) {
	s.requests = append(s.requests, req)
	return &innerpb.UploadMaterialResponse{FileUrl: "https://cdn.example.test/" + req.GetFileName()}, nil
}

func TestDecodeCompositeDataURL(t *testing.T) {
	data := []byte("image-bytes")
	value := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	got, contentType, err := decodeCompositeDataURL(value, 1024)
	if err != nil {
		t.Fatalf("decodeCompositeDataURL() error = %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("decoded data = %q, want %q", got, data)
	}
	if contentType != "image/png" {
		t.Fatalf("content type = %q, want image/png", contentType)
	}
}

func TestDecodeCompositeDataURLRejectsOversize(t *testing.T) {
	value := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("12345"))
	if _, _, err := decodeCompositeDataURL(value, 4); err == nil {
		t.Fatal("decodeCompositeDataURL() error = nil, want size error")
	}
}

func TestPrepareCompositeMaterialPayloadUploadsReferencesAndMask(t *testing.T) {
	stub := &compositeMaterialClientStub{}
	originalFactory := newCompositeMaterialClient
	newCompositeMaterialClient = func(string) compositeMaterialClient { return stub }
	t.Cleanup(func() { newCompositeMaterialClient = originalFactory })

	encoded := base64.StdEncoding.EncodeToString([]byte("image"))
	h := &ModelAPIGatewayHandler{cfg: &config.Config{CompositeMaterial: config.CompositeMaterialConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    9100,
		AppID:   "iapp_test",
		Token:   "secret",
	}}}
	payload := map[string]any{
		"image_urls": []any{"https://example.test/reference.png", "data:image/png;base64," + encoded},
		"mask_url":   "data:image/jpeg;base64," + encoded,
	}
	got, err := h.prepareCompositeMaterialPayload(context.Background(), &service.APIKey{User: &service.User{AccountID: "acct_test"}}, payload)
	if err != nil {
		t.Fatalf("prepareCompositeMaterialPayload() error = %v", err)
	}
	images := got["image_urls"].([]any)
	if images[0] != "https://example.test/reference.png" || images[1] != "https://cdn.example.test/composite-reference-2.png" {
		t.Fatalf("image_urls = %#v", images)
	}
	if got["mask_url"] != "https://cdn.example.test/composite-mask.jpg" {
		t.Fatalf("mask_url = %q", got["mask_url"])
	}
	if len(stub.requests) != 2 || stub.requests[0].GetAccountId() != "acct_test" || stub.requests[1].GetFileName() != "composite-mask.jpg" {
		t.Fatalf("upload requests = %#v", stub.requests)
	}
}

func TestPrepareCompositeMaterialPayloadAllowsHTTPURLsWithoutConfig(t *testing.T) {
	payload := map[string]any{
		"image_urls": []any{"https://example.test/reference.png"},
		"mask_url":   "https://example.test/mask.png",
	}
	if _, err := (&ModelAPIGatewayHandler{}).prepareCompositeMaterialPayload(context.Background(), nil, payload); err != nil {
		t.Fatalf("prepareCompositeMaterialPayload() error = %v", err)
	}
}
