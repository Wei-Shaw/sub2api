package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

func (a *App) handleImagesGenerations(w http.ResponseWriter, r *http.Request) {
	body, err := a.readRequestBody(w, r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	patched, err := patchGrokMediaJSONBody(body, true)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	a.forwardRawWithFailover(w, r, rawForwardInput{
		Method:      http.MethodPost,
		Path:        "/images/generations",
		Body:        patched,
		ContentType: "application/json",
		Accept:      "application/json",
		Endpoint:    "/v1/images/generations",
	})
}

func (a *App) handleImagesEdits(w http.ResponseWriter, r *http.Request) {
	body, err := a.readRequestBody(w, r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	contentType := r.Header.Get("Content-Type")
	patched := body
	if isMultipart(contentType) {
		patched, err = imageEditMultipartToJSON(contentType, body)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	} else {
		patched, err = patchGrokMediaJSONBody(body, true)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	}
	a.forwardRawWithFailover(w, r, rawForwardInput{
		Method:      http.MethodPost,
		Path:        "/images/edits",
		Body:        patched,
		ContentType: "application/json",
		Accept:      "application/json",
		Endpoint:    "/v1/images/edits",
	})
}

func (a *App) handleVideoGeneration(w http.ResponseWriter, r *http.Request) {
	body, err := a.readRequestBody(w, r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	patched, err := patchGrokMediaJSONBody(body, false)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	a.forwardRawWithFailover(w, r, rawForwardInput{
		Method:      http.MethodPost,
		Path:        "/videos/generations",
		Body:        patched,
		ContentType: "application/json",
		Accept:      "application/json",
		Endpoint:    "/v1/videos/generations",
		OnSuccess: func(accountID int64, body []byte) {
			if requestID := extractVideoRequestID(body); requestID != "" {
				a.store.BindVideo(requestID, accountID)
			}
		},
	})
}

func (a *App) handleVideoStatus(w http.ResponseWriter, r *http.Request) {
	requestID := firstNonEmpty(r.PathValue("request_id"), r.URL.Query().Get("request_id"), r.URL.Query().Get("id"))
	if strings.TrimSpace(requestID) == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "request_id is required")
		return
	}
	preferID := a.store.VideoAccountID(requestID)
	a.forwardRawWithFailover(w, r, rawForwardInput{
		Method:   http.MethodGet,
		Path:     "/videos/" + urlPathEscape(requestID),
		Accept:   "application/json",
		Endpoint: "/v1/videos/{request_id}",
		PreferID: preferID,
	})
}

func patchGrokMediaJSONBody(body []byte, imageEndpoint bool) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid json request body")
	}
	model := strings.TrimSpace(asString(payload["model"]))
	if model == "" {
		return nil, errors.New("model is required")
	}
	if imageEndpoint && model == "grok-imagine" {
		model = "grok-imagine-image-quality"
	}
	payload["model"] = mapModel(model)
	return json.Marshal(payload)
}

func imageEditMultipartToJSON(contentType string, body []byte) ([]byte, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, errors.New("multipart boundary is required")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	payload := map[string]any{}
	images := make([]map[string]string, 0, 2)
	var mask string
	for {
		part, err := reader.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}
		data, err := readMultipartPart(part, 32<<20)
		if err != nil {
			return nil, err
		}
		if part.FileName() != "" {
			upload := multipartUpload{
				FieldName:   name,
				FileName:    part.FileName(),
				ContentType: part.Header.Get("Content-Type"),
				Data:        data,
			}
			if name == "mask" {
				mask = uploadToDataURL(upload)
				continue
			}
			if name == "image" || strings.HasPrefix(name, "image[") || name == "images" {
				images = append(images, map[string]string{"image_url": uploadToDataURL(upload)})
			}
			continue
		}
		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			payload["model"] = value
		case "prompt":
			payload["prompt"] = value
		case "n":
			payload["n"] = value
		case "size":
			payload["size"] = value
		case "image", "image_url":
			if value != "" {
				images = append(images, map[string]string{"image_url": value})
			}
		case "mask", "mask_image_url":
			mask = value
		default:
			if value != "" {
				payload[name] = value
			}
		}
	}
	model := strings.TrimSpace(asString(payload["model"]))
	if model == "" {
		return nil, errors.New("model is required")
	}
	if model == "grok-imagine" {
		model = "grok-imagine-image-quality"
	}
	payload["model"] = mapModel(model)
	if len(images) > 0 {
		payload["image"] = images[0]
		if len(images) > 1 {
			payload["images"] = images
		}
	}
	if mask != "" {
		payload["mask"] = map[string]string{"image_url": mask}
	}
	return json.Marshal(payload)
}

func isMultipart(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}

func extractVideoRequestID(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, path := range [][]string{
		{"request_id"}, {"id"}, {"data", "request_id"}, {"data", "id"}, {"video", "request_id"}, {"video", "id"},
	} {
		if value := nestedString(payload, path...); value != "" {
			return value
		}
	}
	return ""
}

func nestedString(payload map[string]any, path ...string) string {
	var current any = payload
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	return strings.TrimSpace(asString(current))
}

func urlPathEscape(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}
