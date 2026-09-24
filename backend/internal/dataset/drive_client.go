package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
func NewDriveClient(ctx context.Context, credentialsPath string, folderID string) (*DriveClient, error) {
	// Read OAuth2 credentials
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %w", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	// Use token from file; fail fast if missing (production-safe)
	tokenPath := credentialsPath + ".token"
	token, err := tokenFromFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("OAuth token not found at %s (generate token with scripts/dataset-oauth-init.sh before starting server): %w", tokenPath, err)
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

// getTokenFromWeb is removed. Use scripts/dataset-oauth-init.sh to generate token offline.
