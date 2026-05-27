package service

import "strings"

func normalizeGroupModelsListConfig(cfg GroupModelsListConfig) GroupModelsListConfig {
	out := GroupModelsListConfig{Enabled: cfg.EnabledREDACTED
	if len(cfg.Models) == 0 {
		return out
REDACTED

	seen := make(map[string]struct{REDACTED, len(cfg.Models))
	out.Models = make([]string, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
	REDACTED
		if _, ok := seen[model]; ok {
			continue
	REDACTED
		seen[model] = struct{REDACTED{REDACTED
		out.Models = append(out.Models, model)
REDACTED
	if len(out.Models) == 0 {
		out.Models = nil
REDACTED
	return out
REDACTED

func (g *Group) CustomModelsListEnabled() bool {
	return g != nil && g.ModelsListConfig.Enabled && len(g.ModelsListConfig.Models) > 0
REDACTED
