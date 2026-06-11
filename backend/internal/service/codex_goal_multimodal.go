package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	codexGoalMaxAttachments       = 20
	codexGoalMaxAttachmentBytes   = 25 << 20
	codexGoalObjectiveMaxChars    = 4000
	codexGoalAttachmentDirName    = "attachments"
	codexGoalStoredFilesDirName   = "files"
	codexGoalBridgePrivateDirName = ".codex-goal-bridge"
)

type CodexGoalAttachment struct {
	Kind       string
	Source     string
	FileID     string
	Filename   string
	MIMEType   string
	Path       string
	Bytes      int64
	RemoteURL  string
	Detail     string
	FromUpload bool
}

type CodexGoalStoredFile struct {
	ID        string    `json:"id"`
	Object    string    `json:"object"`
	Bytes     int64     `json:"bytes"`
	CreatedAt int64     `json:"created_at"`
	Filename  string    `json:"filename"`
	Purpose   string    `json:"purpose"`
	MIMEType  string    `json:"mime_type,omitempty"`
	Path      string    `json:"path,omitempty"`
	Created   time.Time `json:"-"`
}

type codexGoalAttachmentSpec struct {
	Kind      string
	Source    string
	Filename  string
	MIMEType  string
	DataURL   string
	Base64    string
	RemoteURL string
	FileID    string
	Detail    string
}

func (s *CodexGoalBridgeService) StoreUploadedFile(ctx context.Context, filename, contentType, purpose string, body io.Reader) (*CodexGoalStoredFile, error) {
	if !s.IsEnabled() {
		return nil, codexGoalBridgeError(404, "codex_goal_bridge_disabled", "Codex goal bridge is disabled")
	}
	cwd := codexGoalBridgeCWD(s.cfg.Gateway.CodexGoalBridge.CWD)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := readCodexGoalLimitedBytes(body, codexGoalMaxAttachmentBytes)
	if err != nil {
		return nil, codexGoalBridgeError(400, "codex_goal_file_upload_too_large", err.Error())
	}
	if len(data) == 0 {
		return nil, codexGoalBridgeError(400, "codex_goal_file_upload_empty", "uploaded file is empty")
	}
	fileID := "file_codex_goal_" + randomCodexGoalHex(12)
	safeName := safeCodexGoalFilename(filename, contentType, "upload")
	dir := filepath.Join(codexGoalBridgePrivateDir(cwd), codexGoalStoredFilesDirName, fileID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, codexGoalBridgeError(500, "codex_goal_file_store_failed", err.Error())
	}
	path := filepath.Join(dir, safeName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, codexGoalBridgeError(500, "codex_goal_file_store_failed", err.Error())
	}
	now := time.Now().UTC()
	stored := &CodexGoalStoredFile{
		ID:        fileID,
		Object:    "file",
		Bytes:     int64(len(data)),
		CreatedAt: now.Unix(),
		Filename:  safeName,
		Purpose:   strings.TrimSpace(purpose),
		MIMEType:  normalizeCodexGoalMIMEType(contentType),
		Path:      path,
		Created:   now,
	}
	meta, _ := json.MarshalIndent(stored, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), meta, 0600); err != nil {
		return nil, codexGoalBridgeError(500, "codex_goal_file_store_failed", err.Error())
	}
	return stored, nil
}

func (s *CodexGoalBridgeService) loadStoredFile(fileID string) (*CodexGoalStoredFile, error) {
	cwd := codexGoalBridgeCWD(s.cfg.Gateway.CodexGoalBridge.CWD)
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, codexGoalBridgeError(400, "codex_goal_file_id_required", "file_id is required")
	}
	if !codexGoalSafeID(fileID) {
		return nil, codexGoalBridgeError(400, "codex_goal_file_id_invalid", "file_id is invalid")
	}
	metaPath := filepath.Join(codexGoalBridgePrivateDir(cwd), codexGoalStoredFilesDirName, fileID, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, codexGoalBridgeError(400, "codex_goal_file_not_found", "file_id was not found")
	}
	var stored CodexGoalStoredFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, codexGoalBridgeError(500, "codex_goal_file_metadata_invalid", err.Error())
	}
	if strings.TrimSpace(stored.Path) == "" {
		stored.Path = filepath.Join(filepath.Dir(metaPath), stored.Filename)
	}
	if _, err := os.Stat(stored.Path); err != nil {
		return nil, codexGoalBridgeError(400, "codex_goal_file_not_found", "file_id content was not found")
	}
	return &stored, nil
}

func (s *CodexGoalBridgeService) GetStoredFile(fileID string) (*CodexGoalStoredFile, error) {
	if !s.IsEnabled() {
		return nil, codexGoalBridgeError(404, "codex_goal_bridge_disabled", "Codex goal bridge is disabled")
	}
	return s.loadStoredFile(fileID)
}

func (s *CodexGoalBridgeService) stageCodexGoalRequestAttachments(ctx context.Context, req CodexGoalBridgeRequest, cwd string) ([]CodexGoalAttachment, error) {
	var payload map[string]any
	if err := decodeCodexGoalPayload(req.Body, &payload); err != nil {
		return nil, codexGoalBridgeError(400, "invalid_request_body", err.Error())
	}
	payload = normalizeCodexGoalPayload(payload)
	specs := extractCodexGoalAttachmentSpecs(payload, req.Protocol)
	if len(specs) == 0 {
		return nil, nil
	}
	if len(specs) > codexGoalMaxAttachments {
		return nil, codexGoalBridgeError(400, "codex_goal_too_many_attachments", fmt.Sprintf("Codex goal bridge accepts at most %d multimodal attachments", codexGoalMaxAttachments))
	}
	runDir := filepath.Join(codexGoalBridgePrivateDir(cwd), codexGoalAttachmentDirName, time.Now().UTC().Format("20060102T150405.000000000")+"-"+randomCodexGoalHex(6))
	var attachments []CodexGoalAttachment
	for i, spec := range specs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		att, err := s.stageCodexGoalAttachmentSpec(ctx, runDir, i, spec)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

func (s *CodexGoalBridgeService) stageCodexGoalAttachmentSpec(ctx context.Context, runDir string, index int, spec codexGoalAttachmentSpec) (CodexGoalAttachment, error) {
	kind := strings.TrimSpace(spec.Kind)
	if kind == "" {
		kind = "file"
	}
	if strings.TrimSpace(spec.FileID) != "" {
		stored, err := s.loadStoredFile(spec.FileID)
		if err != nil {
			return CodexGoalAttachment{}, err
		}
		return CodexGoalAttachment{
			Kind:       kind,
			Source:     spec.Source,
			FileID:     stored.ID,
			Filename:   stored.Filename,
			MIMEType:   stored.MIMEType,
			Path:       stored.Path,
			Bytes:      stored.Bytes,
			Detail:     spec.Detail,
			FromUpload: true,
		}, nil
	}

	data, mimeType, remoteURL, err := materializeCodexGoalAttachmentData(ctx, spec)
	if err != nil {
		return CodexGoalAttachment{}, err
	}
	if len(data) == 0 {
		return CodexGoalAttachment{}, codexGoalBridgeError(400, "codex_goal_attachment_empty", "multimodal attachment is empty")
	}
	if err := os.MkdirAll(runDir, 0700); err != nil {
		return CodexGoalAttachment{}, codexGoalBridgeError(500, "codex_goal_attachment_store_failed", err.Error())
	}
	filename := safeCodexGoalFilename(spec.Filename, mimeType, fmt.Sprintf("%s-%02d", kind, index+1))
	path := filepath.Join(runDir, filename)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return CodexGoalAttachment{}, codexGoalBridgeError(500, "codex_goal_attachment_store_failed", err.Error())
	}
	return CodexGoalAttachment{
		Kind:      kind,
		Source:    spec.Source,
		Filename:  filename,
		MIMEType:  mimeType,
		Path:      path,
		Bytes:     int64(len(data)),
		RemoteURL: remoteURL,
		Detail:    spec.Detail,
	}, nil
}

func materializeCodexGoalAttachmentData(ctx context.Context, spec codexGoalAttachmentSpec) ([]byte, string, string, error) {
	if dataURL := strings.TrimSpace(spec.DataURL); dataURL != "" {
		data, mimeType, err := decodeCodexGoalDataURL(dataURL)
		if err != nil {
			return nil, "", "", codexGoalBridgeError(400, "codex_goal_attachment_data_invalid", err.Error())
		}
		return data, codexGoalFirstNonEmpty(normalizeCodexGoalMIMEType(spec.MIMEType), mimeType), "", nil
	}
	if raw := strings.TrimSpace(spec.Base64); raw != "" {
		data, err := decodeCodexGoalBase64(raw)
		if err != nil {
			return nil, "", "", codexGoalBridgeError(400, "codex_goal_attachment_data_invalid", err.Error())
		}
		return data, normalizeCodexGoalMIMEType(spec.MIMEType), "", nil
	}
	if remote := strings.TrimSpace(spec.RemoteURL); remote != "" {
		data, mimeType, err := fetchCodexGoalAttachmentURL(ctx, remote)
		if err != nil {
			return nil, "", "", err
		}
		return data, codexGoalFirstNonEmpty(normalizeCodexGoalMIMEType(spec.MIMEType), mimeType), remote, nil
	}
	return nil, "", "", codexGoalBridgeError(400, "codex_goal_attachment_source_required", "multimodal attachment requires file_data, image_url, file_id, or base64 source")
}

func extractCodexGoalAttachmentSpecs(payload map[string]any, protocol string) []codexGoalAttachmentSpec {
	var specs []codexGoalAttachmentSpec
	walkCodexGoalAttachmentSpecs(payload, protocol, "$", &specs)
	return specs
}

func walkCodexGoalAttachmentSpecs(value any, protocol, path string, specs *[]codexGoalAttachmentSpec) {
	switch v := value.(type) {
	case []any:
		for i, item := range v {
			walkCodexGoalAttachmentSpecs(item, protocol, fmt.Sprintf("%s[%d]", path, i), specs)
		}
	case map[string]any:
		if spec, ok := codexGoalAttachmentSpecFromMap(v, protocol, path); ok {
			*specs = append(*specs, spec)
		}
		for key, item := range v {
			walkCodexGoalAttachmentSpecs(item, protocol, path+"."+key, specs)
		}
	}
}

func codexGoalAttachmentSpecFromMap(m map[string]any, protocol, path string) (codexGoalAttachmentSpec, bool) {
	itemType := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(m["type"])))
	switch itemType {
	case "input_image":
		return codexGoalInputImageSpec(m, path), true
	case "image_url":
		return codexGoalChatImageURLSpec(m, path), true
	case "input_file", "file":
		return codexGoalInputFileSpec(m, path), true
	case "image":
		if protocol == CodexGoalProtocolAnthropic {
			return codexGoalAnthropicSourceSpec("image", m, path), true
		}
	case "document":
		if protocol == CodexGoalProtocolAnthropic {
			return codexGoalAnthropicSourceSpec("file", m, path), true
		}
	}
	if inline, ok := codexGoalFirstPresent(m["inlineData"], m["inline_data"]).(map[string]any); ok {
		return codexGoalGeminiInlineSpec(inline, path), true
	}
	if fileData, ok := codexGoalFirstPresent(m["fileData"], m["file_data"]).(map[string]any); ok {
		return codexGoalGeminiFileSpec(fileData, path), true
	}
	return codexGoalAttachmentSpec{}, false
}

func codexGoalInputImageSpec(m map[string]any, path string) codexGoalAttachmentSpec {
	imageURL := codexGoalImageURLValue(m["image_url"])
	spec := codexGoalAttachmentSpec{
		Kind:      "image",
		Source:    path,
		FileID:    strings.TrimSpace(codexGoalStringFromAny(m["file_id"])),
		Filename:  strings.TrimSpace(codexGoalStringFromAny(m["filename"])),
		MIMEType:  normalizeCodexGoalMIMEType(codexGoalStringFromAny(m["mime_type"])),
		Detail:    strings.TrimSpace(codexGoalStringFromAny(m["detail"])),
		RemoteURL: imageURL,
	}
	if strings.HasPrefix(strings.TrimSpace(imageURL), "data:") {
		spec.DataURL = imageURL
		spec.RemoteURL = ""
	}
	return spec
}

func codexGoalChatImageURLSpec(m map[string]any, path string) codexGoalAttachmentSpec {
	return codexGoalInputImageSpec(map[string]any{
		"image_url": m["image_url"],
		"detail":    codexGoalFirstPresent(m["detail"], nestedCodexGoalValue(m["image_url"], "detail")),
	}, path)
}

func codexGoalInputFileSpec(m map[string]any, path string) codexGoalAttachmentSpec {
	fileData := strings.TrimSpace(codexGoalStringFromAny(codexGoalFirstPresent(m["file_data"], m["data"])))
	spec := codexGoalAttachmentSpec{
		Kind:      "file",
		Source:    path,
		FileID:    strings.TrimSpace(codexGoalStringFromAny(m["file_id"])),
		Filename:  strings.TrimSpace(codexGoalStringFromAny(codexGoalFirstPresent(m["filename"], m["name"]))),
		MIMEType:  normalizeCodexGoalMIMEType(codexGoalStringFromAny(codexGoalFirstPresent(m["mime_type"], m["media_type"]))),
		RemoteURL: strings.TrimSpace(codexGoalStringFromAny(codexGoalFirstPresent(m["file_url"], m["url"]))),
	}
	if strings.HasPrefix(fileData, "data:") {
		spec.DataURL = fileData
	} else {
		spec.Base64 = fileData
	}
	return spec
}

func codexGoalAnthropicSourceSpec(kind string, m map[string]any, path string) codexGoalAttachmentSpec {
	source, _ := m["source"].(map[string]any)
	sourceType := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(source["type"])))
	spec := codexGoalAttachmentSpec{
		Kind:     kind,
		Source:   path,
		Filename: strings.TrimSpace(codexGoalStringFromAny(codexGoalFirstPresent(m["title"], m["name"], m["filename"]))),
		MIMEType: normalizeCodexGoalMIMEType(codexGoalStringFromAny(codexGoalFirstPresent(source["media_type"], source["mime_type"]))),
	}
	switch sourceType {
	case "base64":
		spec.Base64 = strings.TrimSpace(codexGoalStringFromAny(source["data"]))
	case "url":
		spec.RemoteURL = strings.TrimSpace(codexGoalStringFromAny(codexGoalFirstPresent(source["url"], source["uri"])))
	default:
		if data := strings.TrimSpace(codexGoalStringFromAny(source["data"])); strings.HasPrefix(data, "data:") {
			spec.DataURL = data
		}
	}
	return spec
}

func codexGoalGeminiInlineSpec(inline map[string]any, path string) codexGoalAttachmentSpec {
	mimeType := normalizeCodexGoalMIMEType(codexGoalStringFromAny(codexGoalFirstPresent(inline["mimeType"], inline["mime_type"])))
	return codexGoalAttachmentSpec{
		Kind:     codexGoalKindFromMIME(mimeType),
		Source:   path,
		MIMEType: mimeType,
		Base64:   strings.TrimSpace(codexGoalStringFromAny(inline["data"])),
	}
}

func codexGoalGeminiFileSpec(fileData map[string]any, path string) codexGoalAttachmentSpec {
	mimeType := normalizeCodexGoalMIMEType(codexGoalStringFromAny(codexGoalFirstPresent(fileData["mimeType"], fileData["mime_type"])))
	return codexGoalAttachmentSpec{
		Kind:      codexGoalKindFromMIME(mimeType),
		Source:    path,
		MIMEType:  mimeType,
		RemoteURL: strings.TrimSpace(codexGoalStringFromAny(codexGoalFirstPresent(fileData["fileUri"], fileData["file_uri"], fileData["uri"], fileData["url"]))),
	}
}

func codexGoalAttachmentSection(attachments []CodexGoalAttachment) string {
	if len(attachments) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Attachments available in this Codex goal run:\n")
	for i, att := range attachments {
		kind := codexGoalFirstNonEmpty(att.Kind, "file")
		b.WriteString(fmt.Sprintf("%d. %s path=%s", i+1, kind, att.Path))
		if att.Filename != "" {
			b.WriteString(" name=")
			b.WriteString(strconvQuoteForObjective(att.Filename))
		}
		if att.MIMEType != "" {
			b.WriteString(" mime=")
			b.WriteString(att.MIMEType)
		}
		if att.FileID != "" {
			b.WriteString(" file_id=")
			b.WriteString(att.FileID)
		}
		if att.Detail != "" {
			b.WriteString(" detail=")
			b.WriteString(att.Detail)
		}
		b.WriteString("\n")
	}
	b.WriteString("Use these local paths for attached images/files.")
	return b.String()
}

func limitCodexGoalObjective(objective string) string {
	objective = strings.TrimSpace(objective)
	if runeLenCodexGoal(objective) <= codexGoalObjectiveMaxChars {
		return objective
	}
	notice := "[Codex goal bridge shortened this objective to fit the Codex /goal 4000 character limit.]"
	attachmentSection := ""
	if idx := strings.Index(objective, "Attachments available in this Codex goal run:"); idx >= 0 {
		attachmentSection = strings.TrimSpace(objective[idx:])
		objective = strings.TrimSpace(objective[:idx])
	}
	if idx := strings.LastIndex(objective, "Current request:\n"); idx >= 0 {
		objective = strings.TrimSpace(objective[idx:])
	}
	parts := []string{notice}
	if objective != "" {
		parts = append(parts, objective)
	}
	if attachmentSection != "" {
		parts = append(parts, attachmentSection)
	}
	combined := strings.Join(parts, "\n\n")
	if runeLenCodexGoal(combined) <= codexGoalObjectiveMaxChars {
		return combined
	}
	return fitCodexGoalObjectiveHeadTail(combined, codexGoalObjectiveMaxChars)
}

func fitCodexGoalObjectiveHeadTail(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	omission := "\n\n[... shortened ...]\n\n"
	omissionRunes := []rune(omission)
	if maxRunes <= len(omissionRunes) {
		return string(runes[:maxRunes])
	}
	head := (maxRunes - len(omissionRunes)) * 2 / 3
	tail := maxRunes - len(omissionRunes) - head
	if head < 0 {
		head = 0
	}
	if tail < 0 {
		tail = 0
	}
	return string(runes[:head]) + omission + string(runes[len(runes)-tail:])
}

func runeLenCodexGoal(value string) int {
	return len([]rune(value))
}

func codexGoalMultimodalPlaceholderFromMap(m map[string]any) string {
	itemType := strings.ToLower(strings.TrimSpace(codexGoalStringFromAny(m["type"])))
	switch itemType {
	case "input_image", "image_url", "image":
		return codexGoalMultimodalPlaceholder("Image", m)
	case "input_file", "file", "document":
		return codexGoalMultimodalPlaceholder("File", m)
	}
	if inline, ok := codexGoalFirstPresent(m["inlineData"], m["inline_data"]).(map[string]any); ok {
		return codexGoalMultimodalPlaceholder(codexGoalKindTitleFromMIME(codexGoalStringFromAny(codexGoalFirstPresent(inline["mimeType"], inline["mime_type"]))), inline)
	}
	if fileData, ok := codexGoalFirstPresent(m["fileData"], m["file_data"]).(map[string]any); ok {
		return codexGoalMultimodalPlaceholder(codexGoalKindTitleFromMIME(codexGoalStringFromAny(codexGoalFirstPresent(fileData["mimeType"], fileData["mime_type"]))), fileData)
	}
	return ""
}

func codexGoalMultimodalPlaceholder(title string, m map[string]any) string {
	var parts []string
	if filename := strings.TrimSpace(codexGoalStringFromAny(codexGoalFirstPresent(m["filename"], m["name"], m["title"]))); filename != "" {
		parts = append(parts, "filename="+filename)
	}
	if fileID := strings.TrimSpace(codexGoalStringFromAny(m["file_id"])); fileID != "" {
		parts = append(parts, "file_id="+fileID)
	}
	if mimeType := normalizeCodexGoalMIMEType(codexGoalStringFromAny(codexGoalFirstPresent(m["mime_type"], m["media_type"], m["mimeType"]))); mimeType != "" {
		parts = append(parts, "mime_type="+mimeType)
	}
	suffix := ""
	if len(parts) > 0 {
		suffix = ": " + strings.Join(parts, ", ")
	}
	return "[" + strings.TrimSpace(title) + " attachment" + suffix + "]"
}

func decodeCodexGoalDataURL(raw string) ([]byte, string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "data:") {
		return nil, "", fmt.Errorf("data URL is required")
	}
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return nil, "", fmt.Errorf("data URL is missing comma separator")
	}
	meta := raw[len("data:"):comma]
	payload := raw[comma+1:]
	mimeType := ""
	isBase64 := false
	for i, part := range strings.Split(meta, ";") {
		if i == 0 && strings.TrimSpace(part) != "" {
			mimeType = normalizeCodexGoalMIMEType(part)
			continue
		}
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
		}
	}
	if !isBase64 {
		decoded, err := url.QueryUnescape(payload)
		if err != nil {
			return nil, "", err
		}
		return []byte(decoded), mimeType, nil
	}
	data, err := decodeCodexGoalBase64(payload)
	if err != nil {
		return nil, "", err
	}
	return data, mimeType, nil
}

func decodeCodexGoalBase64(raw string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			return -1
		}
		return r
	}, raw)
	if decoded, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	return base64.RawURLEncoding.DecodeString(cleaned)
}

func fetchCodexGoalAttachmentURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", codexGoalBridgeError(400, "codex_goal_attachment_url_unsupported", "attachment URL must be http:// or https://")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", codexGoalBridgeError(400, "codex_goal_attachment_url_invalid", err.Error())
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", codexGoalBridgeError(400, "codex_goal_attachment_fetch_failed", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", codexGoalBridgeError(400, "codex_goal_attachment_fetch_failed", fmt.Sprintf("attachment URL returned HTTP %d", resp.StatusCode))
	}
	data, err := readCodexGoalLimitedBytes(resp.Body, codexGoalMaxAttachmentBytes)
	if err != nil {
		return nil, "", codexGoalBridgeError(400, "codex_goal_attachment_too_large", err.Error())
	}
	return data, normalizeCodexGoalMIMEType(resp.Header.Get("Content-Type")), nil
}

func readCodexGoalLimitedBytes(r io.Reader, limit int64) ([]byte, error) {
	var b bytes.Buffer
	n, err := io.Copy(&b, io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, fmt.Errorf("attachment exceeds %d bytes", limit)
	}
	return b.Bytes(), nil
}

func codexGoalBridgeCWD(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd = "."
	}
	if !filepath.IsAbs(cwd) {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
	}
	return cwd
}

func codexGoalBridgePrivateDir(cwd string) string {
	return filepath.Join(codexGoalBridgeCWD(cwd), codexGoalBridgePrivateDirName)
}

var codexGoalUnsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeCodexGoalFilename(filename, contentType, fallback string) string {
	filename = strings.TrimSpace(filepath.Base(filename))
	filename = codexGoalUnsafeFilenameChars.ReplaceAllString(filename, "_")
	filename = strings.Trim(filename, "._-")
	if filename == "" {
		filename = fallback
	}
	if filepath.Ext(filename) == "" {
		if ext := codexGoalExtensionForMIME(contentType); ext != "" {
			filename += ext
		}
	}
	if len(filename) > 120 {
		ext := filepath.Ext(filename)
		stem := strings.TrimSuffix(filename, ext)
		if len(stem) > 100 {
			stem = stem[:100]
		}
		filename = stem + ext
	}
	return filename
}

func codexGoalExtensionForMIME(contentType string) string {
	contentType = normalizeCodexGoalMIMEType(contentType)
	if contentType == "" {
		return ""
	}
	if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	default:
		return ""
	}
}

func normalizeCodexGoalMIMEType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		contentType = mediaType
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

func codexGoalKindFromMIME(mimeType string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return "image"
	}
	return "file"
}

func codexGoalKindTitleFromMIME(mimeType string) string {
	if codexGoalKindFromMIME(mimeType) == "image" {
		return "Image"
	}
	return "File"
}

func codexGoalImageURLValue(value any) string {
	if m, ok := value.(map[string]any); ok {
		return strings.TrimSpace(codexGoalStringFromAny(m["url"]))
	}
	return strings.TrimSpace(codexGoalStringFromAny(value))
}

func nestedCodexGoalValue(value any, key string) any {
	if m, ok := value.(map[string]any); ok {
		return m[key]
	}
	return nil
}

func codexGoalSafeID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func randomCodexGoalHex(n int) string {
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func strconvQuoteForObjective(value string) string {
	if strings.ContainsAny(value, " \t\n\r\"") {
		return strconv.Quote(value)
	}
	return value
}
