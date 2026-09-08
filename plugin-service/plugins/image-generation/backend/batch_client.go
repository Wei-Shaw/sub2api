package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maxBatchImageContentBytes = 20 * 1024 * 1024

type batchSubmitRequest struct {
	Model            string            `json:"model"`
	TaskName         string            `json:"task_name,omitempty"`
	Provider         string            `json:"provider,omitempty"`
	Items            []batchSubmitItem `json:"items"`
	ResponseMimeType string            `json:"response_mime_type,omitempty"`
	AspectRatio      string            `json:"aspect_ratio,omitempty"`
	ImageSize        string            `json:"image_size,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

type batchSubmitItem struct {
	CustomID        string                `json:"custom_id"`
	Prompt          string                `json:"prompt"`
	OutputCount     int                   `json:"output_count"`
	ReferenceImages []batchReferenceInput `json:"reference_images,omitempty"`
}

type batchReferenceInput struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	MimeType string `json:"mime_type"`
	Data     []byte `json:"data,omitempty"`
	FileURI  string `json:"file_uri,omitempty"`
}

type batchJob struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

type batchItem struct {
	CustomID   string            `json:"custom_id"`
	Status     string            `json:"status"`
	ImageCount int               `json:"image_count"`
	Error      *batchPublicError `json:"error,omitempty"`
}

type batchPublicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type batchItemsResponse struct {
	Data    []batchItem `json:"data"`
	HasMore bool        `json:"has_more"`
}

type BatchClient struct {
	httpClient *http.Client
}

func NewBatchClient(httpClient *http.Client) *BatchClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &BatchClient{httpClient: httpClient}
}

func (c *BatchClient) Submit(ctx context.Context, baseURL, apiKey, idempotencyKey string, payload batchSubmitRequest) (*batchJob, error) {
	var result batchJob
	err := c.doJSON(ctx, http.MethodPost, batchEndpoint(baseURL), apiKey, idempotencyKey, payload, &result)
	return &result, err
}

func (c *BatchClient) Get(ctx context.Context, baseURL, apiKey, batchID string) (*batchJob, error) {
	var result batchJob
	err := c.doJSON(ctx, http.MethodGet, batchEndpoint(baseURL, batchID), apiKey, "", nil, &result)
	return &result, err
}

func (c *BatchClient) ListItems(ctx context.Context, baseURL, apiKey, batchID string) (*batchItemsResponse, error) {
	var result batchItemsResponse
	err := c.doJSON(ctx, http.MethodGet, batchEndpoint(baseURL, batchID, "items"), apiKey, "", nil, &result)
	return &result, err
}

func (c *BatchClient) GetItemContent(ctx context.Context, baseURL, apiKey, batchID, customID string, imageIndex int) ([]byte, string, error) {
	requestURL := batchEndpoint(baseURL, batchID, "items", customID, "content") + "?image_index=" + strconv.Itoa(imageIndex)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, "", err
	}
	setBatchAuthorization(req, apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", readBatchHTTPError(resp)
	}
	mimeType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", errors.New("batch item content is not an image")
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxBatchImageContentBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(content) > maxBatchImageContentBytes {
		return nil, "", errors.New("batch item content exceeds size limit")
	}
	return content, mimeType, nil
}

func (c *BatchClient) Cancel(ctx context.Context, baseURL, apiKey, batchID string) (*batchJob, error) {
	var result batchJob
	err := c.doJSON(ctx, http.MethodPost, batchEndpoint(baseURL, batchID, "cancel"), apiKey, "", struct{}{}, &result)
	return &result, err
}

func (c *BatchClient) doJSON(ctx context.Context, method, requestURL, apiKey, idempotencyKey string, payload any, target any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return err
	}
	setBatchAuthorization(req, apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return readBatchHTTPError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func setBatchAuthorization(req *http.Request, apiKey string) {
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
}

func batchEndpoint(baseURL string, segments ...string) string {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/v1/images/batches"
	for _, segment := range segments {
		endpoint += "/" + url.PathEscape(segment)
	}
	return endpoint
}

func readBatchHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	message := extractUpstreamErrorMessage(body)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	return &UpstreamHTTPError{StatusCode: resp.StatusCode, Message: message}
}
