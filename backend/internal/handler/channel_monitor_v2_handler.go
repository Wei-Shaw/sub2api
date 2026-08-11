package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ChannelMonitorV2Handler struct {
	service *service.ChannelMonitorV2Service
REDACTED

func NewChannelMonitorV2Handler(svc *service.ChannelMonitorV2Service) *ChannelMonitorV2Handler {
	return &ChannelMonitorV2Handler{service: svcREDACTED
REDACTED

// channelMonitorV2IsAdmin is true when the request already passed admin auth
// (shared Dimensions/Errors handlers serve both user and admin route groups).
func channelMonitorV2IsAdmin(c *gin.Context) bool {
	role, ok := middleware.GetUserRoleFromContext(c)
	return ok && role == service.RoleAdmin
REDACTED

func (h *ChannelMonitorV2Handler) GetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, cfg)
REDACTED

func (h *ChannelMonitorV2Handler) UpdateConfig(c *gin.Context) {
	var input service.ChannelMonitorV2Config
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "invalid channel monitor v2 config")
		return
REDACTED
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "user not found in context")
		return
REDACTED
	updated, err := h.service.UpdateConfig(c.Request.Context(), input, input.Version, subject.UserID)
	if err != nil {
		if errors.Is(err, service.ErrChannelMonitorV2ConfigConflict) {
			response.Error(c, http.StatusConflict, err.Error())
			return
	REDACTED
		if errors.Is(err, service.ErrChannelMonitorV2InvalidConfig) {
			response.BadRequest(c, err.Error())
			return
	REDACTED
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, updated)
REDACTED

func (h *ChannelMonitorV2Handler) Dimensions(c *gin.Context) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
REDACTED
	result, err := h.service.Dimensions(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	// Admin and user share this handler; only non-admin responses strip volume.
	if !channelMonitorV2IsAdmin(c) {
		service.RedactChannelMonitorV2Dimensions(result)
REDACTED
	response.Success(c, result)
REDACTED

func (h *ChannelMonitorV2Handler) Snapshot(c *gin.Context)      { h.snapshot(c, false) REDACTED
func (h *ChannelMonitorV2Handler) AdminSnapshot(c *gin.Context) { h.snapshot(c, true) REDACTED
func (h *ChannelMonitorV2Handler) Models(c *gin.Context)        { h.models(c, false) REDACTED
func (h *ChannelMonitorV2Handler) AdminModels(c *gin.Context)   { h.models(c, true) REDACTED
func (h *ChannelMonitorV2Handler) Matrix(c *gin.Context)        { h.matrix(c, false) REDACTED
func (h *ChannelMonitorV2Handler) AdminMatrix(c *gin.Context)   { h.matrix(c, true) REDACTED
func (h *ChannelMonitorV2Handler) Users(c *gin.Context)         { h.users(c, false) REDACTED
func (h *ChannelMonitorV2Handler) AdminUsers(c *gin.Context)    { h.users(c, true) REDACTED

func (h *ChannelMonitorV2Handler) snapshot(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
REDACTED
	result, err := h.service.Snapshot(c.Request.Context(), filter, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *ChannelMonitorV2Handler) models(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
REDACTED
	result, err := h.service.Models(c.Request.Context(), filter, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *ChannelMonitorV2Handler) matrix(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
REDACTED
	groupBy, err := service.ParseChannelMonitorV2GroupBy(c.Query("group_by"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED
	result, err := h.service.Matrix(c.Request.Context(), filter, groupBy, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *ChannelMonitorV2Handler) Errors(c *gin.Context) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
REDACTED
	result, err := h.service.ErrorsForViewer(c.Request.Context(), filter, channelMonitorV2IsAdmin(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *ChannelMonitorV2Handler) users(c *gin.Context, admin bool) {
	filter, ok := h.parseFilter(c)
	if !ok {
		return
REDACTED
	subject, exists := middleware.GetAuthSubjectFromContext(c)
	if !exists {
		response.Error(c, http.StatusUnauthorized, "user not found in context")
		return
REDACTED
	result, err := h.service.Users(c.Request.Context(), filter, subject.UserID, admin)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *ChannelMonitorV2Handler) parseFilter(c *gin.Context) (service.ChannelMonitorV2Filter, bool) {
	groups, err := parseChannelMonitorV2GroupIDs(queryList(c, "group_id"))
	if err != nil {
		response.BadRequest(c, "invalid group_id")
		return service.ChannelMonitorV2Filter{REDACTED, false
REDACTED
	filter, err := h.service.ParseFilter(c.Query("range"), queryList(c, "platform"), queryList(c, "model"), groups)
	if err != nil {
		response.BadRequest(c, err.Error())
		return service.ChannelMonitorV2Filter{REDACTED, false
REDACTED
	return filter, true
REDACTED

func queryList(c *gin.Context, key string) []string {
	values := c.QueryArray(key)
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
		REDACTED
	REDACTED
REDACTED
	return result
REDACTED

func parseChannelMonitorV2GroupIDs(values []string) ([]int64, error) {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("invalid group id")
	REDACTED
		result = append(result, id)
REDACTED
	return result, nil
REDACTED
