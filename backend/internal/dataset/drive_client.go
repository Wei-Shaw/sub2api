package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// DriveClient handles Google Drive uploads for dataset collection.
type DriveClient struct {
	service     *drive.Service
	folderID    string
	tokenSource *persistingTokenSource
}

// NewDriveClient creates a new Google Drive client with OAuth2 credentials.
// File-based credentials use the legacy <credentials>.token convention.
// Inline JSON credentials must use NewDriveClientWithTokenPath.
func NewDriveClient(ctx context.Context, credentialsSource string, folderID string) (*DriveClient, error) {
	return NewDriveClientWithTokenPath(ctx, credentialsSource, "", folderID)
}

// NewDriveClientWithTokenPath creates a Google Drive client from either an
// OAuth client JSON file path or inline JSON content. Inline JSON requires an
// explicit token path because it has no stable filesystem location.
func NewDriveClientWithTokenPath(ctx context.Context, credentialsSource, tokenPath, folderID string) (*DriveClient, error) {
	return NewDriveClientWithTokenPaths(ctx, credentialsSource, tokenPath, "", folderID)
}

// NewDriveClientWithTokenPaths separates the read-only token seed from the
// writable persistent refresh state. If no state path is configured, refreshed
// tokens are atomically saved back to the resolved token path.
func NewDriveClientWithTokenPaths(ctx context.Context, credentialsSource, tokenPath, tokenStatePath, folderID string) (*DriveClient, error) {
	b, inline, err := credentialBytes(credentialsSource)
	if err != nil {
		return nil, err
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	resolvedTokenPath, err := resolveTokenPath(credentialsSource, tokenPath, inline)
	if err != nil {
		return nil, err
	}
	resolvedTokenStatePath := strings.TrimSpace(tokenStatePath)
	if resolvedTokenStatePath == "" {
		resolvedTokenStatePath = resolvedTokenPath
	}

	// Prefer the persistent refresh state on restart; otherwise seed from the
	// deployment-provided token, which may be mounted read-only.
	token, err := tokenFromPaths(resolvedTokenPath, resolvedTokenStatePath)
	if err != nil {
		return nil, fmt.Errorf("OAuth token not found at %s (provision an offline OAuth token before starting the server): %w", resolvedTokenPath, err)
	}

	tokenSource, err := newPersistingTokenSource(ctx, config, token, resolvedTokenStatePath)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize Google Drive OAuth token source: %w", err)
	}
	client := newOAuthHTTPClient(ctx, tokenSource)
	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive service: %w", err)
	}

	return &DriveClient{
		service:     service,
		folderID:    folderID,
		tokenSource: tokenSource,
	}, nil
}

func credentialBytes(source string) ([]byte, bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, false, fmt.Errorf("dataset Google Drive credentials are not configured")
	}
	if json.Valid([]byte(source)) {
		return []byte(source), true, nil
	}

	b, err := os.ReadFile(source)
	if err != nil {
		return nil, false, fmt.Errorf("unable to read credentials file: %w", err)
	}
	return b, false, nil
}

func resolveTokenPath(credentialsSource, configuredTokenPath string, inline bool) (string, error) {
	if tokenPath := strings.TrimSpace(configuredTokenPath); tokenPath != "" {
		return tokenPath, nil
	}
	if inline {
		return "", fmt.Errorf("OAuth token path is required when credentials are provided as inline JSON; set DATASET_GOOGLE_DRIVE_TOKEN_PATH")
	}
	return strings.TrimSpace(credentialsSource) + ".token", nil
}

func credentialSourceKind(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "unset"
	}
	if json.Valid([]byte(source)) {
		return "inline-json"
	}
	return "file"
}

// UploadBatch uploads a batch of dataset entries as a JSONL file to Google Drive.
func (dc *DriveClient) UploadBatch(ctx context.Context, entries []DatasetEntry) (string, error) {
	if len(entries) == 0 {
		return "", fmt.Errorf("no entries to upload")
	}

	// Build JSONL content
	var buf bytes.Buffer
	for _, entry := range entries {
		data, err := entry.ToJSON()
		if err != nil {
			log.Printf("[Dataset] Failed to serialize entry: %v", err)
			continue
		}
		if _, err := buf.Write(data); err != nil {
			return "", fmt.Errorf("failed to build JSONL batch: %w", err)
		}
		if err := buf.WriteByte('\n'); err != nil {
			return "", fmt.Errorf("failed to terminate JSONL entry: %w", err)
		}
	}

	// Create filename with timestamp
	filename := fmt.Sprintf("dataset_%s.jsonl", time.Now().UTC().Format("20060102_150405"))

	// Build file metadata
	file := &drive.File{
		Name:     filename,
		MimeType: "application/x-ndjson",
	}
	if dc.folderID != "" {
		file.Parents = []string{dc.folderID}
	}

	// Upload
	upload := func() (*drive.File, error) {
		return dc.service.Files.Create(file).
			Context(ctx).
			Media(bytes.NewReader(buf.Bytes())).
			Fields("id, name, webViewLink").
			Do()
	}
	res, err := upload()
	if err != nil && dc.tokenSource != nil && isInvalidDriveTokenError(err) {
		if _, refreshErr := dc.tokenSource.ForceRefresh(); refreshErr != nil {
			return "", fmt.Errorf("failed to refresh Drive token after invalid_token response: %w", refreshErr)
		}
		res, err = upload()
	}
	if err != nil {
		return "", fmt.Errorf("failed to upload to Drive: %w", safeDriveError(err))
	}

	log.Printf("[Dataset] Uploaded batch: %s (%d entries, %d bytes)", res.Name, len(entries), buf.Len())
	return res.Id, nil
}

func isInvalidDriveTokenError(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusUnauthorized {
		return false
	}
	if hasInvalidTokenChallenge(apiErr.Header.Get("WWW-Authenticate")) || hasInvalidTokenBody(apiErr.Body) {
		return true
	}
	for _, detail := range apiErr.Errors {
		if strings.EqualFold(strings.TrimSpace(detail.Reason), "invalid_token") {
			return true
		}
	}
	return false
}

func hasInvalidTokenChallenge(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, challenge := range strings.Split(value, ",") {
		for _, field := range strings.Fields(challenge) {
			key, candidate, ok := strings.Cut(field, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "error") {
				continue
			}
			candidate = strings.Trim(strings.TrimSpace(candidate), "\"")
			if strings.EqualFold(candidate, "invalid_token") {
				return true
			}
		}
	}
	return false
}

func hasInvalidTokenBody(body string) bool {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return false
	}

	var errorString string
	if err := json.Unmarshal(envelope.Error, &errorString); err == nil {
		return strings.EqualFold(strings.TrimSpace(errorString), "invalid_token")
	}

	var errorObject struct {
		Message string `json:"message"`
		Reason  string `json:"reason"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(envelope.Error, &errorObject); err != nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(errorObject.Message), "invalid_token") ||
		strings.EqualFold(strings.TrimSpace(errorObject.Reason), "invalid_token") {
		return true
	}
	for _, detail := range errorObject.Errors {
		if strings.EqualFold(strings.TrimSpace(detail.Reason), "invalid_token") {
			return true
		}
	}
	return false
}

func safeDriveError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code > 0 {
		return fmt.Errorf("drive request failed with HTTP %d", apiErr.Code)
	}
	return errors.New("drive request failed")
}

// tokenFromFile retrieves a token from a local file.
func tokenFromFile(file string) (tok *oauth2.Token, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	tok = &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func tokenFromPaths(seedPath, statePath string) (*oauth2.Token, error) {
	if strings.TrimSpace(statePath) != "" {
		if token, err := tokenFromFile(statePath); err == nil {
			return token, nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read persistent OAuth token: %w", err)
		}
	}
	return tokenFromFile(seedPath)
}

// getTokenFromWeb is removed. Provision OAuth tokens through an offline operator procedure.
