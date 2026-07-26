package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type PasskeyHandler struct {
	passkeys    *service.PasskeyService
	authService *service.AuthService
	settingSvc  *service.SettingService
REDACTED

func NewPasskeyHandler(
	passkeys *service.PasskeyService,
	authService *service.AuthService,
	settingService *service.SettingService,
) *PasskeyHandler {
	return &PasskeyHandler{
		passkeys:    passkeys,
		authService: authService,
		settingSvc:  settingService,
REDACTED
REDACTED

type passkeyOptionsResponse struct {
	SessionToken string `json:"session_token"`
	Options      any    `json:"options"`
REDACTED

type passkeyFinishRequest struct {
	SessionToken string          `json:"session_token" binding:"required"`
	Name         string          `json:"name,omitempty"`
	Credential   json.RawMessage `json:"credential" binding:"required"`
REDACTED

type passkeyRenameRequest struct {
	Name string `json:"name" binding:"required"`
REDACTED

const passkeyFinishBodyMaxBytes = 64 * 1024

// BeginLogin starts a usernameless, discoverable-credential login ceremony.
func (h *PasskeyHandler) BeginLogin(c *gin.Context) {
	if !h.requirePasskeysEnabled(c) {
		return
REDACTED
	assertion, token, err := h.passkeys.BeginLogin(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, passkeyOptionsResponse{SessionToken: token, Options: assertionREDACTED)
REDACTED

// FinishLogin validates a passkey assertion and creates a normal Sub2API token
// session. User verification is mandatory, so a successful passkey assertion
// already supplies phishing-resistant multi-factor authentication and does not
// enter the separate TOTP challenge flow.
func (h *PasskeyHandler) FinishLogin(c *gin.Context) {
	if !h.requirePasskeysEnabled(c) {
		return
REDACTED
	req, ok := bindPasskeyFinishRequest(c)
	if !ok {
		return
REDACTED
	credentialRequest := cloneRequestWithJSON(c.Request, req.Credential)
	user, err := h.passkeys.FinishLogin(c.Request.Context(), req.SessionToken, credentialRequest)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	if err = h.ensureBackendModeAllowsUser(c.Request.Context(), user); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	middleware2.SetAuditActor(c, user.ID, user.Email)
	c.Set("auth_method", service.AuditAuthMethodPasskey)
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	respondWithTokenPair(c, h.authService, user)
REDACTED

func (h *PasskeyHandler) BeginRegistration(c *gin.Context) {
	if !h.requirePasskeysEnabled(c) {
		return
REDACTED
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED
	creation, token, err := h.passkeys.BeginRegistration(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, passkeyOptionsResponse{SessionToken: token, Options: creationREDACTED)
REDACTED

func (h *PasskeyHandler) FinishRegistration(c *gin.Context) {
	if !h.requirePasskeysEnabled(c) {
		return
REDACTED
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED
	req, valid := bindPasskeyFinishRequest(c)
	if !valid {
		return
REDACTED
	credentialRequest := cloneRequestWithJSON(c.Request, req.Credential)
	credential, err := h.passkeys.FinishRegistration(
		c.Request.Context(),
		subject.UserID,
		req.SessionToken,
		req.Name,
		credentialRequest,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, credential)
REDACTED

func (h *PasskeyHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
REDACTED
	credentials, err := h.passkeys.List(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, credentials)
REDACTED

func (h *PasskeyHandler) Rename(c *gin.Context) {
	subject, credentialID, ok := passkeyMutationTarget(c)
	if !ok {
		return
REDACTED
	var req passkeyRenameRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		response.BadRequest(c, "Passkey name is required")
		return
REDACTED
	if err := h.passkeys.Rename(c.Request.Context(), subject.UserID, credentialID, req.Name); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"success": trueREDACTED)
REDACTED

func (h *PasskeyHandler) Delete(c *gin.Context) {
	subject, credentialID, ok := passkeyMutationTarget(c)
	if !ok {
		return
REDACTED
	if err := h.passkeys.Delete(c.Request.Context(), subject.UserID, credentialID); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"success": trueREDACTED)
REDACTED

func (h *PasskeyHandler) requirePasskeysEnabled(c *gin.Context) bool {
	if h.settingSvc == nil {
		response.ErrorFrom(c, service.ErrPasskeysDisabled)
		return false
REDACTED
	enabled, err := h.settingSvc.PasskeyEnabled(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return false
REDACTED
	if !enabled {
		response.ErrorFrom(c, service.ErrPasskeysDisabled)
REDACTED
	return enabled
REDACTED

func (h *PasskeyHandler) ensureBackendModeAllowsUser(ctx context.Context, user *service.User) error {
	if err := ensureLoginUserActive(user); err != nil {
		return err
REDACTED
	if h.settingSvc == nil || !h.settingSvc.IsBackendModeEnabled(ctx) || user.IsAdmin() {
		return nil
REDACTED
	return infraerrors.Forbidden("BACKEND_MODE_ADMIN_ONLY", "Backend mode is active. Only admin login is allowed.")
REDACTED

func bindPasskeyFinishRequest(c *gin.Context) (*passkeyFinishRequest, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, passkeyFinishBodyMaxBytes)
	var req passkeyFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Credential) == 0 {
		response.BadRequest(c, "Invalid passkey response")
		return nil, false
REDACTED
	return &req, true
REDACTED

func cloneRequestWithJSON(original *http.Request, payload []byte) *http.Request {
	request := original.Clone(original.Context())
	request.Body = io.NopCloser(bytes.NewReader(payload))
	request.ContentLength = int64(len(payload))
	request.Header = original.Header.Clone()
	request.Header.Set("Content-Type", "application/json")
	return request
REDACTED

func passkeyMutationTarget(c *gin.Context) (middleware2.AuthSubject, int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return middleware2.AuthSubject{REDACTED, 0, false
REDACTED
	credentialID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || credentialID <= 0 {
		response.BadRequest(c, "Invalid passkey ID")
		return middleware2.AuthSubject{REDACTED, 0, false
REDACTED
	return subject, credentialID, true
REDACTED
