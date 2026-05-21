package service

import "context"

// ExternalAuthProvider defines the interface for external authentication plugins (e.g., LDAP, AD).
type ExternalAuthProvider interface {
	// Login attempts to authenticate a user and returns the mapped/upserted local User.
	// The core AuthService is responsible for generating JWT tokens for the returned User.
	Login(ctx context.Context, identifier, password string) (*User, error)

	// TestConnection validates the connection to the external identity provider.
	TestConnection(ctx context.Context) error

	// SyncNow performs an immediate synchronization of users from the external provider.
	SyncNow(ctx context.Context) (*LDAPSyncResult, error)

	// Start initializes any background workers (e.g., periodic sync).
	Start()

	// Stop gracefully shuts down background workers.
	Stop()
}
