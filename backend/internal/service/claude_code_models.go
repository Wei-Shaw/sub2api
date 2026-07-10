package service

import "strings"

// normalizeClaudeCodeDefaultModels trims whitespace from each tier mapping.
// Empty values are preserved: an empty tier means "do not emit the
// corresponding ANTHROPIC_DEFAULT_*_MODEL" for that group, so upstreams that
// natively understand Claude model names need no configuration.
func normalizeClaudeCodeDefaultModels(cfg ClaudeCodeDefaultModels) ClaudeCodeDefaultModels {
	return ClaudeCodeDefaultModels{
		Haiku:  strings.TrimSpace(cfg.Haiku),
		Sonnet: strings.TrimSpace(cfg.Sonnet),
		Opus:   strings.TrimSpace(cfg.Opus),
	}
}
