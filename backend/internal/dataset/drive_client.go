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

	// Use token from file or start OAuth flow
	tokenPath := credentialsPath + ".token"
	token, err := tokenFromFile(tokenPath)
	if err != nil {
		token, err = getTokenFromWeb(config)
		if err != nil {
			return nil, fmt.Errorf("unable to get token: %w", err)
		}
		saveToken(tokenPath, token)
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
		buf.Write(data)
		buf.WriteByte('\n')
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
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	f, err := os.Create(path)
	if err != nil {
		log.Printf("[Dataset] Unable to cache oauth token: %v", err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// getTokenFromWeb initiates the OAuth flow in the browser.
func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Printf("[Dataset] Go to the following link in your browser:\n%v\n", authURL)
	log.Printf("[Dataset] Enter authorization code: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %w", err)
	}
	return tok, nil
}
