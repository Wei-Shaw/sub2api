package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type BatchImageHandler struct {
	service  *service.BatchImagePublicService
	download *service.BatchImageDownloadService
	cleanup  *service.BatchImageCleanupService
REDACTED

func NewBatchImageHandler(service *service.BatchImagePublicService, download *service.BatchImageDownloadService, cleanup *service.BatchImageCleanupService) *BatchImageHandler {
	return &BatchImageHandler{service: service, download: download, cleanup: cleanupREDACTED
REDACTED

func (h *BatchImageHandler) Submit(c *gin.Context) {
	var req service.BatchImageSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		batchImageError(c, service.ErrBatchImageInvalidItems)
		return
REDACTED
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	got, err := h.service.Submit(c.Request.Context(), owner, req, c.GetHeader("Idempotency-Key"))
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.JSON(http.StatusOK, got)
REDACTED

func (h *BatchImageHandler) Get(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	got, err := h.service.Get(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.JSON(http.StatusOK, got)
REDACTED

func (h *BatchImageHandler) List(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	limit, _ := strconv.Atoi(c.Query("limit"))
	got, err := h.service.List(c.Request.Context(), owner, service.BatchImageJobsQuery{
		Status:     c.Query("status"),
		TaskName:   c.Query("task_name"),
		Downloaded: c.Query("downloaded"),
		From:       c.Query("from"),
		To:         c.Query("to"),
		Limit:      limit,
		Cursor:     c.Query("cursor"),
REDACTED)
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.JSON(http.StatusOK, got)
REDACTED

func (h *BatchImageHandler) Models(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	got, err := h.service.ListModels(c.Request.Context(), owner)
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.JSON(http.StatusOK, got)
REDACTED

func (h *BatchImageHandler) Items(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	limit, _ := strconv.Atoi(c.Query("limit"))
	got, err := h.service.ListItems(c.Request.Context(), owner, c.Param("id"), service.BatchImageItemsQuery{
		Status: c.Query("status"),
		Limit:  limit,
		Cursor: c.Query("cursor"),
REDACTED)
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.JSON(http.StatusOK, got)
REDACTED

func (h *BatchImageHandler) Cancel(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	got, err := h.service.Cancel(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.JSON(http.StatusOK, got)
REDACTED

func (h *BatchImageHandler) ItemContent(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	imageIndex := 0
	if raw := c.Query("image_index"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			batchImageError(c, service.ErrBatchImageItemImageIndexOutOfRange)
			return
	REDACTED
		imageIndex = parsed
REDACTED
	stream, err := h.download.OpenItemContent(c.Request.Context(), owner, c.Param("id"), c.Param("custom_id"), imageIndex)
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	defer func() { _ = stream.Reader.Close() REDACTED()

	c.Header("Content-Type", stream.ContentType)
	c.Header("Content-Disposition", service.BatchImageContentDispositionAttachment(stream.Filename))
	c.Header("Cache-Control", "private, max-age=300")
	c.Header("X-Content-Type-Options", "nosniff")
	if stream.ContentLength != nil && *stream.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(*stream.ContentLength, 10))
REDACTED
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream.Reader); err != nil {
		return
REDACTED
	_ = h.service.MarkDownloaded(c.Request.Context(), owner, c.Param("id"))
REDACTED

func (h *BatchImageHandler) Download(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	maxItems, _ := strconv.Atoi(c.Query("max_items"))

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", service.BatchImageContentDispositionAttachment(c.Param("id")+".zip"))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	result, err := h.download.StreamZip(c.Request.Context(), owner, c.Param("id"), service.BatchImageZipOptions{
		Status:          c.Query("status"),
		MaxItems:        maxItems,
		IncludeManifest: true,
REDACTED, c.Writer)
	if err != nil {
		if result == nil || !c.Writer.Written() {
			batchImageError(c, err)
	REDACTED
		return
REDACTED
	_ = h.service.MarkDownloaded(c.Request.Context(), owner, c.Param("id"))
REDACTED

func (h *BatchImageHandler) DeleteRecord(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	if err := h.service.DeleteRecord(c.Request.Context(), owner, c.Param("id")); err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.Status(http.StatusNoContent)
REDACTED

func (h *BatchImageHandler) DeleteOutputs(c *gin.Context) {
	owner, ok := batchImageOwnerFromContext(c)
	if !ok {
		batchImageError(c, infraerrors.New(http.StatusUnauthorized, "API_KEY_REQUIRED", "API key is required"))
		return
REDACTED
	got, err := h.cleanup.DeleteOutputsForOwner(c.Request.Context(), owner, c.Param("id"))
	if err != nil {
		batchImageError(c, err)
		return
REDACTED
	c.JSON(http.StatusOK, got)
REDACTED

func batchImageOwnerFromContext(c *gin.Context) (service.BatchImageOwner, bool) {
	apiKey, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.ID <= 0 || apiKey.UserID <= 0 {
		return service.BatchImageOwner{REDACTED, false
REDACTED
	return service.BatchImageOwner{
		UserID:   apiKey.UserID,
		APIKeyID: apiKey.ID,
		GroupID:  apiKey.GroupID,
REDACTED, true
REDACTED

func batchImageError(c *gin.Context, err error) {
	status := infraerrors.Code(err)
	code := infraerrors.Reason(err)
	message := infraerrors.Message(err)
	if err == nil {
		status = http.StatusInternalServerError
		code = "INTERNAL_ERROR"
		message = "internal error"
REDACTED
	if status == 0 || (status == http.StatusInternalServerError && strings.TrimSpace(code) == "") {
		status = http.StatusInternalServerError
		code = "INTERNAL_ERROR"
		message = "internal error"
REDACTED
	if errors.Is(err, service.ErrBatchImageJobNotFound) {
		status = http.StatusNotFound
		code = "BATCH_IMAGE_NOT_FOUND"
		message = "batch image job not found"
REDACTED
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    "invalid_request_error",
			"code":    code,
			"message": message,
	REDACTED,
REDACTED)
REDACTED
