package sora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
)

// UploadCharacterVideo uploads a character video and returns cameo ID.
func (c *Client) UploadCharacterVideo(ctx context.Context, opts RequestOptions, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("video data empty")
REDACTED
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writeMultipartFile(writer, "file", "video.mp4", "video/mp4", data); err != nil {
		return "", err
REDACTED
	if err := writer.WriteField("timestamps", "0,3"); err != nil {
		return "", err
REDACTED
	if err := writer.Close(); err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/characters/upload", opts, &buf, writer.FormDataContentType(), false)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "id"), nil
REDACTED

// GetCameoStatus returns cameo processing status.
func (c *Client) GetCameoStatus(ctx context.Context, opts RequestOptions, cameoID string) (map[string]any, error) {
	if cameoID == "" {
		return nil, errors.New("cameo id empty")
REDACTED
	return c.doRequest(ctx, "GET", "/project_y/cameos/in_progress/"+cameoID, opts, nil, "", false)
REDACTED

// DownloadCharacterImage downloads character avatar image data.
func (c *Client) DownloadCharacterImage(ctx context.Context, opts RequestOptions, imageURL string) ([]byte, error) {
	if c.upstream == nil {
		return nil, errors.New("upstream is nil")
REDACTED
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("User-Agent", defaultDesktopUA)
	resp, err := c.upstream.DoWithTLS(req, opts.ProxyURL, opts.AccountID, opts.AccountConcurrency, c.enableTLSFingerprint)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download image failed: %d", resp.StatusCode)
REDACTED
	return io.ReadAll(resp.Body)
REDACTED

// UploadCharacterImage uploads character avatar and returns asset pointer.
func (c *Client) UploadCharacterImage(ctx context.Context, opts RequestOptions, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("image data empty")
REDACTED
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writeMultipartFile(writer, "file", "profile.webp", "image/webp", data); err != nil {
		return "", err
REDACTED
	if err := writer.WriteField("use_case", "profile"); err != nil {
		return "", err
REDACTED
	if err := writer.Close(); err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/project_y/file/upload", opts, &buf, writer.FormDataContentType(), false)
	if err != nil {
		return "", err
REDACTED
	return stringFromJSON(resp, "asset_pointer"), nil
REDACTED

// FinalizeCharacter finalizes character creation and returns character ID.
func (c *Client) FinalizeCharacter(ctx context.Context, opts RequestOptions, cameoID, username, displayName, assetPointer string) (string, error) {
	payload := map[string]any{
		"cameo_id":               cameoID,
		"username":               username,
		"display_name":           displayName,
		"profile_asset_pointer":  assetPointer,
		"instruction_set":        nil,
		"safety_instruction_set": nil,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	resp, err := c.doRequest(ctx, "POST", "/characters/finalize", opts, bytes.NewReader(body), "application/json", false)
	if err != nil {
		return "", err
REDACTED
	if character, ok := resp["character"].(map[string]any); ok {
		if id, ok := character["character_id"].(string); ok {
			return id, nil
	REDACTED
REDACTED
	return "", nil
REDACTED

// SetCharacterPublic marks character as public.
func (c *Client) SetCharacterPublic(ctx context.Context, opts RequestOptions, cameoID string) error {
	payload := map[string]any{"visibility": "public"REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return err
REDACTED
	_, err = c.doRequest(ctx, "POST", "/project_y/cameos/by_id/"+cameoID+"/update_v2", opts, bytes.NewReader(body), "application/json", false)
	return err
REDACTED

// DeleteCharacter deletes a character by ID.
func (c *Client) DeleteCharacter(ctx context.Context, opts RequestOptions, characterID string) error {
	if characterID == "" {
		return nil
REDACTED
	_, err := c.doRequest(ctx, "DELETE", "/project_y/characters/"+characterID, opts, nil, "", false)
	return err
REDACTED

func writeMultipartFile(writer *multipart.Writer, field, filename, contentType string, data []byte) error {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	if contentType != "" {
		header.Set("Content-Type", contentType)
REDACTED
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
REDACTED
	_, err = part.Write(data)
	return err
REDACTED
