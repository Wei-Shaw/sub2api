package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	DefaultDataManagementAgentSocketPath = "/tmp/sub2api-datamanagement.sock"
	LegacyBackupAgentSocketPath          = "/tmp/sub2api-backup.sock"

	DataManagementDeprecatedReason         = "DATA_MANAGEMENT_DEPRECATED"
	DataManagementAgentSocketMissingReason = "DATA_MANAGEMENT_AGENT_SOCKET_MISSING"
	DataManagementAgentUnavailableReason   = "DATA_MANAGEMENT_AGENT_UNAVAILABLE"

	// Deprecated: keep old names for compatibility.
	DefaultBackupAgentSocketPath   = DefaultDataManagementAgentSocketPath
	BackupAgentSocketMissingReason = DataManagementAgentSocketMissingReason
	BackupAgentUnavailableReason   = DataManagementAgentUnavailableReason
)

var (
	ErrDataManagementDeprecated = infraerrors.ServiceUnavailable(
		DataManagementDeprecatedReason,
		"data management feature is deprecated",
	)
	ErrDataManagementAgentSocketMissing = infraerrors.ServiceUnavailable(
		DataManagementAgentSocketMissingReason,
		"data management agent socket is missing",
	)
	ErrDataManagementAgentUnavailable = infraerrors.ServiceUnavailable(
		DataManagementAgentUnavailableReason,
		"data management agent is unavailable",
	)

	// Deprecated: keep old names for compatibility.
	ErrBackupAgentSocketMissing = ErrDataManagementAgentSocketMissing
	ErrBackupAgentUnavailable   = ErrDataManagementAgentUnavailable
)

type DataManagementAgentHealth struct {
	Enabled    bool
	Reason     string
	SocketPath string
	Agent      *DataManagementAgentInfo
}

type DataManagementAgentInfo struct {
	Status        string
	Version       string
	UptimeSeconds int64
}

type DataManagementService struct {
	socketPath string
}

func NewDataManagementService() *DataManagementService {
	return NewDataManagementServiceWithOptions(DefaultDataManagementAgentSocketPath, 500*time.Millisecond)
}

func NewDataManagementServiceWithOptions(socketPath string, dialTimeout time.Duration) *DataManagementService {
	_ = dialTimeout
	path := strings.TrimSpace(socketPath)
	if path == "" {
		path = DefaultDataManagementAgentSocketPath
	}
	return &DataManagementService{
		socketPath: path,
	}
}

func (s *DataManagementService) SocketPath() string {
	if s == nil || strings.TrimSpace(s.socketPath) == "" {
		return DefaultDataManagementAgentSocketPath
	}
	return s.socketPath
}

func (s *DataManagementService) GetAgentHealth(ctx context.Context) DataManagementAgentHealth {
	_ = ctx
	return DataManagementAgentHealth{
		Enabled:    false,
		Reason:     DataManagementDeprecatedReason,
		SocketPath: s.SocketPath(),
	}
}

func (s *DataManagementService) EnsureAgentEnabled(ctx context.Context) error {
	_ = ctx
	return ErrDataManagementDeprecated.WithMetadata(map[string]string{"socket_path": s.SocketPath()})
}
