package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Setting-domain errors used by persistence and application layers.
var (
	ErrSettingNotFound = infraerrors.NotFound("SETTING_NOT_FOUND", "setting not found")
)

// Setting is a key/value system configuration row.
type Setting struct {
	ID        int64
	Key       string
	Value     string
	UpdatedAt time.Time
}
