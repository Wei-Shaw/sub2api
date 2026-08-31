package cursor

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	availableModelsBodyLimit = 8 << 20
	availableModelsTimeout   = 12 * time.Second
)

// AvailableModel is one picker entry from aiserver.v1.AiService/AvailableModels.
type AvailableModel struct {
	Name            string
	DisplayName     string
	ServerModelName string
	DefaultOn       bool
	Hidden          bool
	Aliases         []string
	LegacySlugs     []string
}

// ModelIDs returns canonical picker slugs in catalog order.
func ModelIDs(models []AvailableModel) []string {
	ids := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.Name)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// AvailableModels fetches the live Cursor model picker for these credentials.
// The request flags match Cursor 3.16's refreshDefaultModels call.
func (c *Client) AvailableModels(ctx context.Context) ([]AvailableModel, error) {
	if c == nil {
		return nil, fmt.Errorf("cursor: available models: nil client")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, availableModelsTimeout)
	defer cancel()

	// api2 unary methods speak application/proto (see EstablishSession), not Connect envelopes.
	models, err := c.availableModelsOnce(fetchCtx, false)
	if err == nil && len(models) > 0 {
		return models, nil
	}
	fallback, fallbackErr := c.availableModelsOnce(fetchCtx, true)
	if fallbackErr == nil && len(fallback) > 0 {
		return fallback, nil
	}
	if err != nil {
		return nil, err
	}
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	return nil, fmt.Errorf("cursor: available models: empty catalog")
}

func (c *Client) availableModelsOnce(ctx context.Context, connectRPC bool) ([]AvailableModel, error) {
	payload := encodeAvailableModelsRequest()
	body := payload
	contentType := "application/proto"
	if connectRPC {
		frame, err := EncodeFrame(payload, false)
		if err != nil {
			return nil, fmt.Errorf("cursor: available models: encode frame: %w", err)
		}
		body = frame
		contentType = "application/connect+proto"
	}

	base := c.APIBaseURL
	if base == "" {
		base = BaseURLAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+EndpointModels, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	headers := BuildHeaders(c.Creds)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("content-type", contentType)
	if connectRPC {
		req.Header.Set("connect-protocol-version", "1")
	} else {
		req.Header.Del("connect-protocol-version")
		req.Header.Set("accept-encoding", "gzip")
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = NewHTTP2Transport()
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cursor: available models: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, availableModelsBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("cursor: available models: read: %w", err)
	}
	if int64(len(raw)) > availableModelsBodyLimit {
		return nil, fmt.Errorf("cursor: available models: response too large")
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("cursor: available models status %d: %s", resp.StatusCode, msg)
	}
	return parseAvailableModelsHTTPBody(raw)
}

func encodeAvailableModelsRequest() []byte {
	var w ProtobufWriter
	// Match Cursor 3.16 refreshDefaultModels: excludeMaxNamedModels, useModelParameters,
	// useReactModelPicker. Field numbers from aiserver.v1.AvailableModelsRequest.
	w.Bool(3, true)  // exclude_max_named_models
	w.Bool(5, true)  // use_model_parameters
	w.Bool(11, true) // use_react_model_picker
	return w.Result()
}

func parseAvailableModelsHTTPBody(body []byte) ([]AvailableModel, error) {
	payload, err := maybeGunzip(body)
	if err != nil {
		return nil, fmt.Errorf("cursor: available models: gzip: %w", err)
	}
	if looksLikeConnectFrame(payload) {
		frame, err := DecodeFrame(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("cursor: available models: decode frame: %w", err)
		}
		if msg := ConnectErrorJSON(frame); msg != "" {
			return nil, fmt.Errorf("cursor: available models: %s", msg)
		}
		payload = frame.Payload
	}
	models := ParseAvailableModelsResponse(payload)
	if len(models) == 0 {
		prefix := payload
		if len(prefix) > 24 {
			prefix = prefix[:24]
		}
		return nil, fmt.Errorf("cursor: available models: empty catalog (len=%d prefix=%x)", len(payload), prefix)
	}
	return models, nil
}

func maybeGunzip(body []byte) ([]byte, error) {
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return body, nil
	}
	gr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(io.LimitReader(gr, availableModelsBodyLimit))
}

func looksLikeConnectFrame(body []byte) bool {
	if len(body) < 5 {
		return false
	}
	flags := body[0]
	if flags&^(FrameFlagGzip|FrameFlagEndStream) != 0 {
		return false
	}
	length := int(body[1])<<24 | int(body[2])<<16 | int(body[3])<<8 | int(body[4])
	return length >= 0 && 5+length <= len(body)
}

// ParseAvailableModelsResponse decodes aiserver.v1.AvailableModelsResponse.
func ParseAvailableModelsResponse(data []byte) []AvailableModel {
	if len(data) == 0 {
		return nil
	}
	var models []AvailableModel
	var names []string
	r := NewProtobufReader(data)
	for {
		f, err := r.Next()
		if f == nil || err != nil {
			break
		}
		if f.WireType != WireBytes {
			continue
		}
		switch f.Num {
		case 1:
			if s := strings.TrimSpace(string(f.Data)); s != "" {
				names = append(names, s)
			}
		case 2:
			model := parseAvailableModel(f.Data)
			if model.Name == "" || model.Hidden {
				continue
			}
			models = append(models, model)
		}
	}
	if len(models) > 0 {
		return models
	}
	out := make([]AvailableModel, 0, len(names))
	for _, name := range names {
		out = append(out, AvailableModel{Name: name, ServerModelName: name})
	}
	return out
}

func parseAvailableModel(data []byte) AvailableModel {
	var model AvailableModel
	r := NewProtobufReader(data)
	for {
		f, err := r.Next()
		if f == nil || err != nil {
			break
		}
		switch f.Num {
		case 1:
			if f.WireType == WireBytes {
				model.Name = strings.TrimSpace(string(f.Data))
			}
		case 2:
			model.DefaultOn = f.Varint != 0
		case 17:
			if f.WireType == WireBytes {
				model.DisplayName = string(f.Data)
			}
		case 18:
			if f.WireType == WireBytes {
				model.ServerModelName = string(f.Data)
			}
		case 35:
			model.Hidden = f.Varint != 0
		case 36:
			if f.WireType == WireBytes {
				if s := strings.TrimSpace(string(f.Data)); s != "" {
					model.LegacySlugs = append(model.LegacySlugs, s)
				}
			}
		case 37:
			if f.WireType == WireBytes {
				if s := strings.TrimSpace(string(f.Data)); s != "" {
					model.Aliases = append(model.Aliases, s)
				}
			}
		}
	}
	if model.ServerModelName == "" {
		model.ServerModelName = model.Name
	}
	if model.DisplayName == "" {
		model.DisplayName = model.Name
	}
	return model
}
