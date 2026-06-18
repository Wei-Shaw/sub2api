package repository

import "github.com/Wei-Shaw/Nub2api/internal/pkg/geminicli"

// NewGeminiDriveClient creates a concrete DriveClient for Google Drive API operations.
// Returned as geminicli.DriveClient interface for DI (Strategy A).
func NewGeminiDriveClient() geminicli.DriveClient {
	return geminicli.NewDriveClient()
}
