package service

import (
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var codexDefaultModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)

// normalizeCodexDefaultModel accepts an optional model ID for exported OpenAI
// Codex configurations. Empty restores the built-in default; quotes, controls,
// and other configuration syntax are rejected before any settings are written.
func normalizeCodexDefaultModel(raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model != "" && !codexDefaultModelPattern.MatchString(model) {
		return "", infraerrors.BadRequest("INVALID_CODEX_DEFAULT_MODEL", "Codex default model must be a model ID of at most 200 ASCII characters")
	}
	return model, nil
}
