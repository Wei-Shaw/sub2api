package plugin

import (
	"errors"
	"regexp"
)

// pluginNamePattern is the canonical character set for plugin names.
//
//   - lowercase alphanumeric + dash + underscore
//   - must start AND end with alnum (rejects "-foo", "foo_", ".."  etc.)
//   - length 1-64
//
// This both rules out path-traversal payloads (no "/", no ".", no "..")
// and produces a name that filepath.Join cannot escape. New-line / NUL /
// control characters / Unicode punctuation are all rejected.
var pluginNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{0,62}[a-z0-9])?$`)

// ErrInvalidPluginName is returned by manager / repository entry points
// when a plugin name fails IsValidPluginName. It is mapped to 400 by
// admin handlers via shared error helpers.
var ErrInvalidPluginName = errors.New("invalid plugin name")

// IsValidPluginName reports whether name conforms to pluginNamePattern.
// Used by:
//   - admin HTTP handlers (parseName / parsePlugin) to reject malicious
//     input at the boundary
//   - manager EnablePlugin / DisablePlugin / RestartPlugin / UpdateConfig
//     as defense in depth against new entry points that bypass handlers
//   - discovery to skip filesystem entries that should not become plugins
//   - repository.Create as a final DB-layer guard
func IsValidPluginName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	return pluginNamePattern.MatchString(name)
}
