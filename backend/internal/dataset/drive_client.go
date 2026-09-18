package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// DriveClient handles Google Drive uploads for dataset collection.
type DriveClient struct {
	service  *drive.Service
	folderID string
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
	b, inline, err := credentialBytes(credentialsSource)
	if err != nil {
		return nil, err
	}

	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	resolvedTokenPath, err := resolveTokenPath(credentialsSource, tokenPath, inline)
	if err != nil {
		return nil, err
	}

	// Use token from file; fail fast if missing (production-safe)
	token, err := tokenFromFile(resolvedTokenPath)
	if err != nil {
		return nil, fmt.Errorf("OAuth token not found at %s (provision an offline OAuth token before starting the server): %w", resolvedTokenPath, err)
	}

	client := config.Client(ctx, token)
	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive service: %w", err)
	}

	return &DriveClient{
		service:  service,
		folderID: folderID,
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
	res, err := dc.service.Files.Create(file).
		Context(ctx).
		Media(&buf).
		Fields("id, name, webViewLink").
		Do()
	if err != nil {
		return "", fmt.Errorf("failed to upload to Drive: %w", err)
	}

	log.Printf("[Dataset] Uploaded batch: %s (%d entries, %d bytes)", res.Name, len(entries), buf.Len())
	return res.Id, nil
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

// getTokenFromWeb is removed. Provision OAuth tokens through an offline operator procedure.
