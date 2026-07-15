package upstreamstation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func (c *Sub2APIConnector) ListAPIKeys(ctx context.Context, baseURL string, session *Session) ([]ManagedKey, error) {
	params := url.Values{"page": {"1"}, "page_size": {"100"}}
	data, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/v1/keys?"+params.Encode(), session)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			Key     string `json:"key"`
			GroupID *int64 `json:"group_id"`
			Status  string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	keys := make([]ManagedKey, 0, len(result.Items))
	for _, item := range result.Items {
		groupKey := ""
		if item.GroupID != nil {
			groupKey = strconv.FormatInt(*item.GroupID, 10)
		}
		keys = append(keys, ManagedKey{ID: strconv.FormatInt(item.ID, 10), Name: item.Name, GroupKey: groupKey, Key: item.Key, Status: item.Status})
	}
	return keys, nil
}

func (c *Sub2APIConnector) CreateAPIKey(ctx context.Context, baseURL string, session *Session, name string, group RemoteGroup) (*ManagedKey, error) {
	groupID, err := strconv.ParseInt(group.Key, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid sub2api group id: %w", err)
	}
	body, err := json.Marshal(map[string]any{"name": strings.TrimSpace(name), "group_id": groupID})
	if err != nil {
		return nil, err
	}
	data, err := c.request(ctx, http.MethodPost, normalizeBaseURL(baseURL)+"/api/v1/keys", body, session)
	if err != nil {
		return nil, err
	}
	var item struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Key     string `json:"key"`
		GroupID *int64 `json:"group_id"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	if item.ID <= 0 {
		return nil, errors.New("sub2api create key returned no id")
	}
	groupKey := group.Key
	if item.GroupID != nil {
		groupKey = strconv.FormatInt(*item.GroupID, 10)
	}
	return &ManagedKey{ID: strconv.FormatInt(item.ID, 10), Name: item.Name, GroupKey: groupKey, Key: item.Key, Status: item.Status}, nil
}

func (c *Sub2APIConnector) RevealAPIKey(ctx context.Context, baseURL string, session *Session, id string) (string, error) {
	data, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/v1/keys/"+url.PathEscape(id), session)
	if err != nil {
		return "", err
	}
	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Key) == "" {
		return "", errors.New("sub2api returned an empty key")
	}
	return result.Key, nil
}

func (c *NewAPIConnector) ListAPIKeys(ctx context.Context, baseURL string, session *Session) ([]ManagedKey, error) {
	data, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/token/?p=1&size=100", session)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Key    string `json:"key"`
			Group  string `json:"group"`
			Status int    `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	keys := make([]ManagedKey, 0, len(result.Items))
	for _, item := range result.Items {
		status := "disabled"
		if item.Status == 1 {
			status = "active"
		}
		keys = append(keys, ManagedKey{ID: strconv.FormatInt(item.ID, 10), Name: item.Name, GroupKey: item.Group, Key: item.Key, Status: status})
	}
	return keys, nil
}

func (c *NewAPIConnector) CreateAPIKey(ctx context.Context, baseURL string, session *Session, name string, group RemoteGroup) (*ManagedKey, error) {
	name = strings.TrimSpace(name)
	body, err := json.Marshal(map[string]any{
		"name":                 name,
		"group":                group.Key,
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"cross_group_retry":    false,
	})
	if err != nil {
		return nil, err
	}
	data, err := c.request(ctx, http.MethodPost, normalizeBaseURL(baseURL)+"/api/token/", body, session)
	if err != nil {
		return nil, err
	}
	var object struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Key    string `json:"key"`
		Group  string `json:"group"`
		Status int    `json:"status"`
	}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &object)
	}
	if object.ID <= 0 && len(data) > 0 {
		var id int64
		if idErr := json.Unmarshal(data, &id); idErr == nil {
			object.ID = id
		}
	}
	if object.ID <= 0 {
		keys, listErr := c.ListAPIKeys(ctx, baseURL, session)
		if listErr != nil {
			return nil, fmt.Errorf("list newapi keys after create: %w", listErr)
		}
		var created *ManagedKey
		for i := range keys {
			candidate := &keys[i]
			if candidate.Name != name || candidate.GroupKey != group.Key {
				continue
			}
			if created == nil {
				created = candidate
				continue
			}
			candidateID, candidateErr := strconv.ParseInt(candidate.ID, 10, 64)
			createdID, createdErr := strconv.ParseInt(created.ID, 10, 64)
			if candidateErr == nil && (createdErr != nil || candidateID > createdID) {
				created = candidate
			}
		}
		if created == nil {
			return nil, errors.New("newapi create key returned no id and the created key was not found")
		}
		key := *created
		key.Key, err = c.RevealAPIKey(ctx, baseURL, session, key.ID)
		if err != nil {
			return nil, err
		}
		return &key, nil
	}
	if object.Name == "" {
		object.Name = name
	}
	if object.Group == "" {
		object.Group = group.Key
	}
	key := &ManagedKey{ID: strconv.FormatInt(object.ID, 10), Name: object.Name, GroupKey: object.Group, Key: object.Key, Status: "active"}
	if strings.TrimSpace(key.Key) == "" || strings.Contains(key.Key, "*") {
		key.Key, err = c.RevealAPIKey(ctx, baseURL, session, key.ID)
		if err != nil {
			return nil, err
		}
	}
	return key, nil
}

func (c *NewAPIConnector) RevealAPIKey(ctx context.Context, baseURL string, session *Session, id string) (string, error) {
	data, err := c.request(ctx, http.MethodPost, normalizeBaseURL(baseURL)+"/api/token/"+url.PathEscape(id)+"/key", nil, session)
	if err != nil {
		return "", err
	}
	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Key) == "" {
		return "", errors.New("newapi returned an empty key")
	}
	return result.Key, nil
}

func (c *NewAPIConnector) request(ctx context.Context, method, requestURL string, body []byte, session *Session) ([]byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	applyNewAPIAuth(req, session)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	return decodeNewAPIResponse(io.LimitReader(resp.Body, 8<<20))
}

func (c *Sub2APIConnector) ListModels(ctx context.Context, baseURL, apiKey, platform string) ([]string, error) {
	return listGatewayModels(ctx, c.http, baseURL, apiKey, platform)
}

func (c *NewAPIConnector) ListModels(ctx context.Context, baseURL, apiKey, platform string) ([]string, error) {
	return listGatewayModels(ctx, c.http, baseURL, apiKey, platform)
}

func listGatewayModels(ctx context.Context, client *http.Client, baseURL, apiKey, platform string) ([]string, error) {
	baseURL = normalizeBaseURL(baseURL)
	endpoint := baseURL + "/v1/models"
	if strings.EqualFold(platform, "gemini") {
		endpoint = baseURL + "/v1beta/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "gemini":
		req.Header.Set("x-goog-api-key", apiKey)
	case "anthropic":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := connectorHTTPClient(client).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model list returned HTTP %d", resp.StatusCode)
	}
	var raw any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	models := uniqueModelIDs(collectModelIDs(raw))
	if len(models) == 0 {
		return nil, errors.New("upstream returned no models")
	}
	return models, nil
}

func collectModelIDs(value any) []string {
	switch typed := value.(type) {
	case []any:
		var out []string
		for _, item := range typed {
			out = append(out, collectModelIDs(item)...)
		}
		return out
	case map[string]any:
		var out []string
		if data, ok := typed["data"]; ok {
			out = append(out, collectModelIDs(data)...)
		}
		if models, ok := typed["models"]; ok {
			out = append(out, collectModelIDs(models)...)
		}
		for _, field := range []string{"id", "name", "model"} {
			if id, ok := typed[field].(string); ok {
				out = append(out, id)
				break
			}
		}
		return out
	case string:
		return []string{typed}
	default:
		return nil
	}
}

func uniqueModelIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(value, "models/"))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
