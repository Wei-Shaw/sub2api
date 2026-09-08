package backend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	hostprincipal "github.com/Wei-Shaw/sub2api/plugin-service/internal/host/principal"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
)

var ErrAPIKeyUnavailable = errors.New("api key is unavailable for image generation")

type APIKeyResolver interface {
	ResolveAny(ctx context.Context, request *http.Request, principal model.CurrentPrincipal, baseURL string, keyID int64) (string, error)
	Resolve(ctx context.Context, request *http.Request, principal model.CurrentPrincipal, baseURL string, keyID int64, modelName string) (string, error)
}

type MainServiceAPIKeyResolver struct {
	httpClient *http.Client
}

type apiKeyEnvelope struct {
	Code int            `json:"code"`
	Data resolvedAPIKey `json:"data"`
}

type resolvedAPIKey struct {
	ID     int64             `json:"id"`
	UserID int64             `json:"user_id"`
	Key    string            `json:"key"`
	Status string            `json:"status"`
	Group  *resolvedKeyGroup `json:"group"`
}

type resolvedKeyGroup struct {
	AllowImageGeneration bool                     `json:"allow_image_generation"`
	ModelsListConfig     resolvedModelsListConfig `json:"models_list_config"`
}

type resolvedModelsListConfig struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models"`
}

func NewMainServiceAPIKeyResolver(client *http.Client) *MainServiceAPIKeyResolver {
	if client == nil {
		client = http.DefaultClient
	}
	return &MainServiceAPIKeyResolver{httpClient: client}
}

func (r *MainServiceAPIKeyResolver) Resolve(ctx context.Context, source *http.Request, principal model.CurrentPrincipal, baseURL string, keyID int64, modelName string) (string, error) {
	key, err := r.resolveKey(ctx, source, principal, baseURL, keyID)
	if err != nil {
		return "", err
	}
	if key.Group.ModelsListConfig.Enabled && !containsModel(key.Group.ModelsListConfig.Models, modelName) {
		return "", ErrAPIKeyUnavailable
	}
	return strings.TrimSpace(key.Key), nil
}

func (r *MainServiceAPIKeyResolver) ResolveAny(ctx context.Context, source *http.Request, principal model.CurrentPrincipal, baseURL string, keyID int64) (string, error) {
	key, err := r.resolveKey(ctx, source, principal, baseURL, keyID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(key.Key), nil
}

func (r *MainServiceAPIKeyResolver) resolveKey(ctx context.Context, source *http.Request, principal model.CurrentPrincipal, baseURL string, keyID int64) (resolvedAPIKey, error) {
	if keyID <= 0 || principal.UserID <= 0 || strings.TrimSpace(baseURL) == "" {
		return resolvedAPIKey{}, ErrAPIKeyUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/keys/"+strconv.FormatInt(keyID, 10), nil)
	if err != nil {
		return resolvedAPIKey{}, err
	}
	hostprincipal.CopyMainSiteCredentials(req, source)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return resolvedAPIKey{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resolvedAPIKey{}, ErrAPIKeyUnavailable
	}
	var envelope apiKeyEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return resolvedAPIKey{}, err
	}
	key := envelope.Data
	if envelope.Code != 0 || key.ID != keyID || key.UserID != principal.UserID || !strings.EqualFold(key.Status, "active") || strings.TrimSpace(key.Key) == "" || key.Group == nil || !key.Group.AllowImageGeneration {
		return resolvedAPIKey{}, ErrAPIKeyUnavailable
	}
	return key, nil
}

func containsModel(models []string, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	for _, candidate := range models {
		if strings.EqualFold(strings.TrimSpace(candidate), modelName) {
			return true
		}
	}
	return false
}
