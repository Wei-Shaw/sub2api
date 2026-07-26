package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Vertex service-account pure Account methods + the pure key parser they need.
// Lifted from internal/service/vertex_service_account.go in Phase 3 (Account BC
// hybrid). The impure token-exchange / HTTP path stays in service.

const vertexDefaultLocation = "us-central1"

// VertexDefaultLocation is exported for the service-layer URL builders that
// still reference the default under the original unexported name.
const VertexDefaultLocation = vertexDefaultLocation

type vertexServiceAccountKey struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"`
}

func (a *Account) IsVertexServiceAccount() bool {
	return a != nil && a.Type == AccountTypeServiceAccount
}

func (a *Account) VertexProjectID() string {
	if a == nil {
		return ""
	}
	if v := strings.TrimSpace(a.GetCredential("project_id")); v != "" {
		return v
	}
	key, err := ParseVertexServiceAccountKey(a)
	if err == nil {
		return strings.TrimSpace(key.ProjectID)
	}
	return ""
}

func (a *Account) VertexLocation(model string) string {
	if a == nil {
		return vertexDefaultLocation
	}
	if model != "" && a.Credentials != nil {
		if raw, ok := a.Credentials["vertex_model_locations"].(map[string]any); ok {
			if loc, ok := raw[model].(string); ok && strings.TrimSpace(loc) != "" {
				return strings.TrimSpace(loc)
			}
		}
	}
	if v := strings.TrimSpace(a.GetCredential("location")); v != "" {
		return v
	}
	if v := strings.TrimSpace(a.GetCredential("vertex_location")); v != "" {
		return v
	}
	return vertexDefaultLocation
}

// VertexServiceAccountKey is the exported form of the parsed service-account
// JSON so the service-layer token exchange can keep using the same shape.
type VertexServiceAccountKey = vertexServiceAccountKey

// ParseVertexServiceAccountKey parses the service-account credentials off an
// Account. Exported so the service-layer token exchange (and batch-image /
// gemini-token paths) can re-export under the original unexported name.
func ParseVertexServiceAccountKey(account *Account) (*vertexServiceAccountKey, error) {
	if account == nil || account.Credentials == nil {
		return nil, errors.New("service account credentials not configured")
	}

	if raw := strings.TrimSpace(account.GetCredential("service_account_json")); raw != "" {
		return parseVertexServiceAccountJSON([]byte(raw))
	}
	if raw := strings.TrimSpace(account.GetCredential("service_account")); raw != "" {
		return parseVertexServiceAccountJSON([]byte(raw))
	}
	if nested, ok := account.Credentials["service_account_json"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		return parseVertexServiceAccountJSON(b)
	}
	if nested, ok := account.Credentials["service_account"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		return parseVertexServiceAccountJSON(b)
	}
	return nil, errors.New("service_account_json not found in credentials")
}

func parseVertexServiceAccountJSON(raw []byte) (*vertexServiceAccountKey, error) {
	var key vertexServiceAccountKey
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, fmt.Errorf("invalid service account json: %w", err)
	}
	if strings.TrimSpace(key.ClientEmail) == "" {
		return nil, errors.New("service account json missing client_email")
	}
	if strings.TrimSpace(key.PrivateKey) == "" {
		return nil, errors.New("service account json missing private_key")
	}
	if strings.TrimSpace(key.ProjectID) == "" {
		return nil, errors.New("service account json missing project_id")
	}
	// Always use the well-known Google token endpoint to prevent SSRF via crafted token_uri.
	// The token URL itself lives in the service layer (exchange path); leave blank here so
	// the service-side exchange can fill it. Historical behaviour set it on parse — keep that
	// by using the well-known constant below.
	key.TokenURI = "https://oauth2.googleapis.com/token"
	return &key, nil
}
