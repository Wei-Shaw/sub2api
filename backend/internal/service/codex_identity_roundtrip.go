package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var codexIdentityResponseContainerPaths = [][]string{
	nil,
	{"client_metadata"},
	{"metadata"},
	{"headers"},
	{"response"},
	{"response", "client_metadata"},
	{"response", "metadata"},
	{"response", "headers"},
	{"response", "error"},
	{"response", "error", "client_metadata"},
	{"response", "error", "metadata"},
	{"response", "error", "headers"},
	{"request"},
	{"request", "client_metadata"},
	{"request", "metadata"},
	{"request", "headers"},
	{"error"},
	{"error", "client_metadata"},
	{"error", "metadata"},
	{"error", "headers"},
}

var codexIdentityJSONFields = map[CodexIdentityKind][]string{
	CodexIdentityInstallation: {"installation_id", "x-codex-installation-id"},
	CodexIdentitySession:      {"session_id", "session-id"},
	CodexIdentityConversation: {"conversation_id"},
	CodexIdentityThread:       {"thread_id", "thread-id", "x-client-request-id"},
	CodexIdentityTurn:         {"turn_id"},
	CodexIdentityWindow:       {"window_id", "x-codex-window-id"},
	CodexIdentityPromptCache:  {"prompt_cache_key"},
	CodexIdentityWorkspace:    {"workspace", "workspace_path", "cwd", "working_directory"},
	CodexIdentityGitRemote:    {"git_remote", "git_remote_url", "remote_url"},
	CodexIdentityGitCommit:    {"git_commit", "git_sha", "commit_sha"},
}

// ApplyCodexIdentityPlanToJSON applies a resolved attempt plan only to known
// request identity fields. Prompt/input content is never traversed.
func ApplyCodexIdentityPlanToJSON(payload []byte, plan *CodexIdentityAttemptPlan) ([]byte, bool, error) {
	if plan == nil || len(plan.RequestMappings) == 0 || len(payload) == 0 {
		return payload, false, nil
	}
	root, err := decodeCodexIdentityJSON(payload)
	if err != nil {
		return payload, false, err
	}
	object, ok := root.(map[string]any)
	if !ok {
		return payload, false, nil
	}
	changed, err := ApplyCodexIdentityPlanToMap(object, plan)
	if err != nil {
		return payload, false, err
	}
	if !changed {
		return payload, false, nil
	}
	rewritten, err := json.Marshal(object)
	if err != nil {
		return payload, false, fmt.Errorf("encode Codex identity request JSON: %w", err)
	}
	return rewritten, true, nil
}

func ApplyCodexIdentityPlanToMap(root map[string]any, plan *CodexIdentityAttemptPlan) (bool, error) {
	if root == nil || plan == nil {
		return false, nil
	}
	metadata, exists, err := codexIdentityRequestObject(root, "client_metadata")
	if err != nil {
		return false, err
	}
	if !exists {
		metadata = make(map[string]any, 12)
		root["client_metadata"] = metadata
	}
	applyCodexIdentityPlanToExistingContainer(metadata, plan)
	deleteCodexIdentityHeaderAliases(metadata)

	set := func(key string, kind CodexIdentityKind) {
		if value := plan.UpstreamValue(kind); value != "" {
			metadata[key] = value
		}
	}
	set("installation_id", CodexIdentityInstallation)
	set("session_id", CodexIdentitySession)
	set("conversation_id", CodexIdentityConversation)
	set("thread_id", CodexIdentityThread)
	set("turn_id", CodexIdentityTurn)
	set("window_id", CodexIdentityWindow)
	applyCodexProfileMetadata(metadata, plan.Profile, true)

	for _, item := range []struct {
		kind CodexIdentityKind
		key  string
	}{
		{CodexIdentityWorkspace, "workspace_path"},
		{CodexIdentityGitRemote, "git_remote_url"},
		{CodexIdentityGitCommit, "git_sha"},
	} {
		if mapping, ok := codexIdentityMappingForKind(plan, item.kind); ok && mapping.ClientValue != "" {
			metadata[item.key] = mapping.UpstreamValue
		}
	}
	if mapping, ok := codexIdentityMappingForKind(plan, CodexIdentityPromptCache); ok && mapping.ClientValue != "" {
		root["prompt_cache_key"] = mapping.UpstreamValue
	}
	if err := applyCodexIdentityPlanToEmbeddedMetadata(metadata, plan); err != nil {
		return false, err
	}
	for _, containerName := range []string{"metadata", "turn_metadata"} {
		container, present, containerErr := codexIdentityRequestObject(root, containerName)
		if containerErr != nil {
			return false, containerErr
		}
		if present {
			applyCodexIdentityPlanToExistingContainer(container, plan)
			applyCodexProfileMetadata(container, plan.Profile, false)
		}
	}
	if request, present, requestErr := codexIdentityRequestObject(root, "request"); requestErr != nil {
		return false, requestErr
	} else if present {
		requestMetadata, metadataPresent, metadataErr := codexIdentityRequestObject(request, "client_metadata")
		if metadataErr != nil {
			return false, metadataErr
		}
		if metadataPresent {
			applyCodexIdentityPlanToExistingContainer(requestMetadata, plan)
			applyCodexProfileMetadata(requestMetadata, plan.Profile, true)
			if err := applyCodexIdentityPlanToEmbeddedMetadata(requestMetadata, plan); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

func deleteCodexIdentityHeaderAliases(metadata map[string]any) {
	for actualKey := range metadata {
		for _, headerAlias := range []string{
			"x-codex-installation-id",
			"x-codex-window-id",
			"session-id",
			"thread-id",
			"x-client-request-id",
		} {
			if strings.EqualFold(strings.TrimSpace(actualKey), headerAlias) {
				delete(metadata, actualKey)
				break
			}
		}
	}
}

func codexIdentityRequestObject(parent map[string]any, key string) (map[string]any, bool, error) {
	value, exists := parent[key]
	if !exists || value == nil {
		return nil, false, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("codex identity request field %s must be an object", key)
	}
	return object, true, nil
}

func applyCodexProfileMetadata(metadata map[string]any, profile CodexResolvedProfile, add bool) {
	values := codexResolvedProfileMetadataValues(profile)
	for key, value := range values {
		if value == "" {
			if _, exists := metadata[key]; exists {
				metadata[key] = ""
			}
			continue
		}
		if _, exists := metadata[key]; add || exists {
			metadata[key] = value
		}
	}
}

func applyCodexIdentityPlanToExistingContainer(container map[string]any, plan *CodexIdentityAttemptPlan) {
	for _, mapping := range plan.RequestMappings {
		for actualKey := range container {
			if codexIdentityFieldMatches(actualKey, codexIdentityJSONFields[mapping.Kind]) {
				container[actualKey] = mapping.UpstreamValue
			}
		}
	}
}

func codexIdentityMappingForKind(plan *CodexIdentityAttemptPlan, kind CodexIdentityKind) (CodexIdentityMapping, bool) {
	if plan == nil {
		return CodexIdentityMapping{}, false
	}
	for _, mapping := range plan.RequestMappings {
		if mapping.Kind == kind {
			return mapping, true
		}
	}
	return CodexIdentityMapping{}, false
}

func applyCodexIdentityPlanToEmbeddedMetadata(metadata map[string]any, plan *CodexIdentityAttemptPlan) error {
	for actualKey, raw := range metadata {
		if !strings.EqualFold(strings.TrimSpace(actualKey), "x-codex-turn-metadata") &&
			!strings.EqualFold(strings.TrimSpace(actualKey), "turn_metadata") {
			continue
		}
		var embedded map[string]any
		stringEncoded := false
		switch typed := raw.(type) {
		case map[string]any:
			embedded = typed
		case string:
			decoded, err := decodeCodexIdentityJSON([]byte(typed))
			if err != nil {
				return fmt.Errorf("decode embedded Codex turn metadata: %w", err)
			}
			embedded, _ = decoded.(map[string]any)
			if embedded == nil {
				return errors.New("embedded Codex turn metadata must be an object")
			}
			stringEncoded = true
		default:
			return errors.New("embedded Codex turn metadata must be an object or JSON string")
		}
		if embedded == nil {
			continue
		}
		for _, item := range []struct {
			kind CodexIdentityKind
			key  string
		}{
			{CodexIdentityInstallation, "installation_id"},
			{CodexIdentitySession, "session_id"},
			{CodexIdentityConversation, "conversation_id"},
			{CodexIdentityThread, "thread_id"},
			{CodexIdentityTurn, "turn_id"},
			{CodexIdentityWindow, "window_id"},
			{CodexIdentityWorkspace, "workspace_path"},
			{CodexIdentityGitRemote, "git_remote_url"},
			{CodexIdentityGitCommit, "git_sha"},
		} {
			if value := plan.UpstreamValue(item.kind); value != "" {
				embedded[item.key] = value
			}
		}
		applyCodexProfileMetadata(embedded, plan.Profile, true)
		if stringEncoded {
			if rebuilt, err := json.Marshal(embedded); err == nil {
				metadata[actualKey] = string(rebuilt)
			} else {
				return fmt.Errorf("encode embedded Codex turn metadata: %w", err)
			}
		}
	}
	return nil
}

// RestoreCodexIdentityJSON restores aliases only at known protocol identity
// locations. It never recursively scans arbitrary output/content objects.
func RestoreCodexIdentityJSON(payload []byte, plan *CodexIdentityAttemptPlan) ([]byte, bool, error) {
	if plan == nil || len(plan.RequestMappings) == 0 || len(payload) == 0 {
		return payload, false, nil
	}
	root, err := decodeCodexIdentityJSON(payload)
	if err != nil {
		return payload, false, err
	}
	changed := restoreCodexIdentityRoot(root, plan)
	if !changed {
		return payload, false, nil
	}
	restored, err := json.Marshal(root)
	if err != nil {
		return payload, false, fmt.Errorf("encode restored Codex identity JSON: %w", err)
	}
	return restored, true, nil
}

// RestoreCodexIdentityWSEvent applies the same structured contract to one WS
// JSON event. The caller remains responsible for frame I/O.
func RestoreCodexIdentityWSEvent(payload []byte, plan *CodexIdentityAttemptPlan) ([]byte, bool, error) {
	return RestoreCodexIdentityJSON(payload, plan)
}

func decodeCodexIdentityJSON(payload []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode Codex identity JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode Codex identity JSON: trailing value")
		}
		return nil, fmt.Errorf("decode Codex identity JSON trailing data: %w", err)
	}
	return root, nil
}

func restoreCodexIdentityRoot(root any, plan *CodexIdentityAttemptPlan) bool {
	object, ok := root.(map[string]any)
	if !ok {
		return false
	}
	changed := false
	for _, path := range codexIdentityResponseContainerPaths {
		container := codexIdentityObjectAtPath(object, path)
		if container == nil {
			continue
		}
		if restoreCodexIdentityContainer(container, plan) {
			changed = true
		}
		if restoreCodexProfileContainer(container, plan) {
			changed = true
		}
		if restoreEmbeddedCodexTurnMetadata(container, plan) {
			changed = true
		}
	}
	return changed
}

func restoreCodexProfileContainer(container map[string]any, plan *CodexIdentityAttemptPlan) bool {
	if container == nil || plan == nil {
		return false
	}
	changed := false
	for _, mapping := range plan.ProfileMappings {
		for actualKey, value := range container {
			if !strings.EqualFold(strings.TrimSpace(actualKey), mapping.Field) {
				continue
			}
			text, ok := value.(string)
			if !ok || text != mapping.UpstreamValue {
				continue
			}
			if !mapping.ClientPresent {
				delete(container, actualKey)
			} else {
				container[actualKey] = mapping.ClientValue
			}
			changed = true
		}
	}
	return changed
}

func codexIdentityObjectAtPath(root map[string]any, path []string) map[string]any {
	current := root
	for _, segment := range path {
		value, ok := current[segment]
		if !ok {
			return nil
		}
		next, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func restoreCodexIdentityContainer(container map[string]any, plan *CodexIdentityAttemptPlan) bool {
	if container == nil || plan == nil {
		return false
	}
	changed := false
	for _, mapping := range plan.RequestMappings {
		if mapping.UpstreamValue == "" || mapping.ClientValue == mapping.UpstreamValue {
			continue
		}
		fields := codexIdentityJSONFields[mapping.Kind]
		for actualKey, value := range container {
			if !codexIdentityFieldMatches(actualKey, fields) {
				continue
			}
			if mapping.ClientValue == "" {
				if codexIdentityValueContainsAlias(value, mapping.UpstreamValue) {
					delete(container, actualKey)
					changed = true
				}
				continue
			}
			restored, fieldChanged := restoreExactCodexIdentityValue(value, mapping.UpstreamValue, mapping.ClientValue)
			if fieldChanged {
				container[actualKey] = restored
				changed = true
			}
		}
	}
	return changed
}

func codexIdentityValueContainsAlias(value any, alias string) bool {
	switch typed := value.(type) {
	case string:
		return typed == alias
	case []any:
		for _, item := range typed {
			if itemString, ok := item.(string); ok && itemString == alias {
				return true
			}
		}
	}
	return false
}

func codexIdentityFieldMatches(actual string, allowed []string) bool {
	for _, name := range allowed {
		if strings.EqualFold(strings.TrimSpace(actual), name) {
			return true
		}
	}
	return false
}

func restoreExactCodexIdentityValue(value any, upstream, client string) (any, bool) {
	switch typed := value.(type) {
	case string:
		if typed == upstream {
			return client, true
		}
	case []any:
		changed := false
		copyValues := append([]any(nil), typed...)
		for i, item := range copyValues {
			if itemString, ok := item.(string); ok && itemString == upstream {
				copyValues[i] = client
				changed = true
			}
		}
		if changed {
			return copyValues, true
		}
	}
	return value, false
}

func restoreEmbeddedCodexTurnMetadata(container map[string]any, plan *CodexIdentityAttemptPlan) bool {
	changed := false
	for actualKey, value := range container {
		if !strings.EqualFold(strings.TrimSpace(actualKey), "x-codex-turn-metadata") &&
			!strings.EqualFold(strings.TrimSpace(actualKey), "turn_metadata") {
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			if restoreCodexIdentityContainer(typed, plan) || restoreCodexProfileContainer(typed, plan) {
				changed = true
			}
		case string:
			metadataRoot, err := decodeCodexIdentityJSON([]byte(typed))
			if err != nil {
				continue
			}
			metadata, ok := metadataRoot.(map[string]any)
			if !ok {
				continue
			}
			identityChanged := restoreCodexIdentityContainer(metadata, plan)
			profileChanged := restoreCodexProfileContainer(metadata, plan)
			if !identityChanged && !profileChanged {
				continue
			}
			rebuilt, err := json.Marshal(metadata)
			if err != nil {
				continue
			}
			container[actualKey] = string(rebuilt)
			changed = true
		}
	}
	return changed
}

type codexSSELine struct {
	content string
	ending  string
	removed bool
}

// RestoreCodexIdentitySSE restores only JSON carried by SSE data fields. Event
// names, comments, keepalives, [DONE], and non-JSON data remain untouched.
func RestoreCodexIdentitySSE(stream []byte, plan *CodexIdentityAttemptPlan) ([]byte, bool, error) {
	if plan == nil || len(plan.RequestMappings) == 0 || len(stream) == 0 {
		return stream, false, nil
	}
	lines := splitCodexSSELines(stream)
	changed := false
	frameStart := 0
	for i := 0; i <= len(lines); i++ {
		if i < len(lines) && lines[i].content != "" {
			continue
		}
		frameChanged, err := restoreCodexIdentitySSEFrame(lines, frameStart, i, plan)
		if err != nil {
			return stream, false, err
		}
		changed = changed || frameChanged
		frameStart = i + 1
	}
	if !changed {
		return stream, false, nil
	}
	var rebuilt strings.Builder
	rebuilt.Grow(len(stream))
	for _, line := range lines {
		if line.removed {
			continue
		}
		_, _ = rebuilt.WriteString(line.content)
		_, _ = rebuilt.WriteString(line.ending)
	}
	return []byte(rebuilt.String()), true, nil
}

func splitCodexSSELines(stream []byte) []codexSSELine {
	lines := make([]codexSSELine, 0, bytes.Count(stream, []byte{'\n'})+1)
	for len(stream) > 0 {
		index := bytes.IndexByte(stream, '\n')
		if index < 0 {
			lines = append(lines, codexSSELine{content: string(stream)})
			break
		}
		content := stream[:index]
		ending := "\n"
		if len(content) > 0 && content[len(content)-1] == '\r' {
			content = content[:len(content)-1]
			ending = "\r\n"
		}
		lines = append(lines, codexSSELine{content: string(content), ending: ending})
		stream = stream[index+1:]
	}
	return lines
}

func restoreCodexIdentitySSEFrame(lines []codexSSELine, start, end int, plan *CodexIdentityAttemptPlan) (bool, error) {
	if start < 0 || start >= end || end > len(lines) {
		return false, nil
	}
	dataIndexes := make([]int, 0, 1)
	dataValues := make([]string, 0, 1)
	for i := start; i < end; i++ {
		value, ok := codexSSEDataValue(lines[i].content)
		if !ok {
			continue
		}
		dataIndexes = append(dataIndexes, i)
		dataValues = append(dataValues, value)
	}
	if len(dataIndexes) == 0 {
		return false, nil
	}
	joined := strings.Join(dataValues, "\n")
	if strings.TrimSpace(joined) == "[DONE]" {
		return false, nil
	}
	restored, changed, err := restoreCodexIdentityJSONData(joined, plan)
	if err != nil || !changed {
		return false, err
	}
	first := dataIndexes[0]
	lines[first].content = "data: " + restored
	for _, index := range dataIndexes[1:] {
		lines[index].removed = true
	}
	return true, nil
}

func codexSSEDataValue(line string) (string, bool) {
	if line == "data" {
		return "", true
	}
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	value := line[len("data:"):]
	value = strings.TrimPrefix(value, " ")
	return value, true
}

func restoreCodexIdentityJSONData(data string, plan *CodexIdentityAttemptPlan) (string, bool, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" {
		return data, false, nil
	}
	if json.Valid([]byte(trimmed)) {
		restored, changed, err := RestoreCodexIdentityJSON([]byte(trimmed), plan)
		return string(restored), changed, err
	}
	documents, ok := splitOpenAIConcatenatedJSONDocuments([]byte(trimmed))
	if !ok || len(documents) == 0 {
		return data, false, nil
	}
	changed := false
	var rebuilt bytes.Buffer
	for _, document := range documents {
		restored, documentChanged, err := RestoreCodexIdentityJSON(document, plan)
		if err != nil {
			return data, false, err
		}
		changed = changed || documentChanged
		_, _ = rebuilt.Write(restored)
	}
	if !changed {
		return data, false, nil
	}
	return rebuilt.String(), true, nil
}
