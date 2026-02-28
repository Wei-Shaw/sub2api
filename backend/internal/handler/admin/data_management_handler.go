package admin

import (
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type DataManagementHandler struct {
	dataManagementService *service.DataManagementService
REDACTED

func NewDataManagementHandler(dataManagementService *service.DataManagementService) *DataManagementHandler {
	return &DataManagementHandler{dataManagementService: dataManagementServiceREDACTED
REDACTED

type TestS3ConnectionRequest struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region" binding:"required"`
	Bucket          string `json:"bucket" binding:"required"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Prefix          string `json:"prefix"`
	ForcePathStyle  bool   `json:"force_path_style"`
	UseSSL          bool   `json:"use_ssl"`
REDACTED

type CreateBackupJobRequest struct {
	BackupType     string `json:"backup_type" binding:"required,oneof=postgres redis full"`
	UploadToS3     bool   `json:"upload_to_s3"`
	S3ProfileID    string `json:"s3_profile_id"`
	PostgresID     string `json:"postgres_profile_id"`
	RedisID        string `json:"redis_profile_id"`
	IdempotencyKey string `json:"idempotency_key"`
REDACTED

type CreateSourceProfileRequest struct {
	ProfileID string                             `json:"profile_id" binding:"required"`
	Name      string                             `json:"name" binding:"required"`
	Config    service.DataManagementSourceConfig `json:"config" binding:"required"`
	SetActive bool                               `json:"set_active"`
REDACTED

type UpdateSourceProfileRequest struct {
	Name   string                             `json:"name" binding:"required"`
	Config service.DataManagementSourceConfig `json:"config" binding:"required"`
REDACTED

type CreateS3ProfileRequest struct {
	ProfileID       string `json:"profile_id" binding:"required"`
	Name            string `json:"name" binding:"required"`
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Prefix          string `json:"prefix"`
	ForcePathStyle  bool   `json:"force_path_style"`
	UseSSL          bool   `json:"use_ssl"`
	SetActive       bool   `json:"set_active"`
REDACTED

type UpdateS3ProfileRequest struct {
	Name            string `json:"name" binding:"required"`
	Enabled         bool   `json:"enabled"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Prefix          string `json:"prefix"`
	ForcePathStyle  bool   `json:"force_path_style"`
	UseSSL          bool   `json:"use_ssl"`
REDACTED

func (h *DataManagementHandler) GetAgentHealth(c *gin.Context) {
	health := h.getAgentHealth(c)
	payload := gin.H{
		"enabled":     health.Enabled,
		"reason":      health.Reason,
		"socket_path": health.SocketPath,
REDACTED
	if health.Agent != nil {
		payload["agent"] = gin.H{
			"status":         health.Agent.Status,
			"version":        health.Agent.Version,
			"uptime_seconds": health.Agent.UptimeSeconds,
	REDACTED
REDACTED
	response.Success(c, payload)
REDACTED

func (h *DataManagementHandler) GetConfig(c *gin.Context) {
	if !h.requireAgentEnabled(c) {
		return
REDACTED
	cfg, err := h.dataManagementService.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, cfg)
REDACTED

func (h *DataManagementHandler) UpdateConfig(c *gin.Context) {
	var req service.DataManagementConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	cfg, err := h.dataManagementService.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, cfg)
REDACTED

func (h *DataManagementHandler) TestS3(c *gin.Context) {
	var req TestS3ConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	result, err := h.dataManagementService.ValidateS3(c.Request.Context(), service.DataManagementS3Config{
		Enabled:         true,
		Endpoint:        req.Endpoint,
		Region:          req.Region,
		Bucket:          req.Bucket,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: req.SecretAccessKey,
		Prefix:          req.Prefix,
		ForcePathStyle:  req.ForcePathStyle,
		UseSSL:          req.UseSSL,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"ok": result.OK, "message": result.MessageREDACTED)
REDACTED

func (h *DataManagementHandler) CreateBackupJob(c *gin.Context) {
	var req CreateBackupJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	req.IdempotencyKey = normalizeBackupIdempotencyKey(c.GetHeader("X-Idempotency-Key"), req.IdempotencyKey)
	if !h.requireAgentEnabled(c) {
		return
REDACTED

	triggeredBy := "admin:unknown"
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		triggeredBy = "admin:" + strconv.FormatInt(subject.UserID, 10)
REDACTED
	job, err := h.dataManagementService.CreateBackupJob(c.Request.Context(), service.DataManagementCreateBackupJobInput{
		BackupType:     req.BackupType,
		UploadToS3:     req.UploadToS3,
		S3ProfileID:    req.S3ProfileID,
		PostgresID:     req.PostgresID,
		RedisID:        req.RedisID,
		TriggeredBy:    triggeredBy,
		IdempotencyKey: req.IdempotencyKey,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"job_id": job.JobID, "status": job.StatusREDACTED)
REDACTED

func (h *DataManagementHandler) ListSourceProfiles(c *gin.Context) {
	sourceType := strings.TrimSpace(c.Param("source_type"))
	if sourceType == "" {
		response.BadRequest(c, "Invalid source_type")
		return
REDACTED
	if sourceType != "postgres" && sourceType != "redis" {
		response.BadRequest(c, "source_type must be postgres or redis")
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	items, err := h.dataManagementService.ListSourceProfiles(c.Request.Context(), sourceType)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"items": itemsREDACTED)
REDACTED

func (h *DataManagementHandler) CreateSourceProfile(c *gin.Context) {
	sourceType := strings.TrimSpace(c.Param("source_type"))
	if sourceType != "postgres" && sourceType != "redis" {
		response.BadRequest(c, "source_type must be postgres or redis")
		return
REDACTED

	var req CreateSourceProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	profile, err := h.dataManagementService.CreateSourceProfile(c.Request.Context(), service.DataManagementCreateSourceProfileInput{
		SourceType: sourceType,
		ProfileID:  req.ProfileID,
		Name:       req.Name,
		Config:     req.Config,
		SetActive:  req.SetActive,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, profile)
REDACTED

func (h *DataManagementHandler) UpdateSourceProfile(c *gin.Context) {
	sourceType := strings.TrimSpace(c.Param("source_type"))
	if sourceType != "postgres" && sourceType != "redis" {
		response.BadRequest(c, "source_type must be postgres or redis")
		return
REDACTED
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Invalid profile_id")
		return
REDACTED

	var req UpdateSourceProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	profile, err := h.dataManagementService.UpdateSourceProfile(c.Request.Context(), service.DataManagementUpdateSourceProfileInput{
		SourceType: sourceType,
		ProfileID:  profileID,
		Name:       req.Name,
		Config:     req.Config,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, profile)
REDACTED

func (h *DataManagementHandler) DeleteSourceProfile(c *gin.Context) {
	sourceType := strings.TrimSpace(c.Param("source_type"))
	if sourceType != "postgres" && sourceType != "redis" {
		response.BadRequest(c, "source_type must be postgres or redis")
		return
REDACTED
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Invalid profile_id")
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	if err := h.dataManagementService.DeleteSourceProfile(c.Request.Context(), sourceType, profileID); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"deleted": trueREDACTED)
REDACTED

func (h *DataManagementHandler) SetActiveSourceProfile(c *gin.Context) {
	sourceType := strings.TrimSpace(c.Param("source_type"))
	if sourceType != "postgres" && sourceType != "redis" {
		response.BadRequest(c, "source_type must be postgres or redis")
		return
REDACTED
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Invalid profile_id")
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	profile, err := h.dataManagementService.SetActiveSourceProfile(c.Request.Context(), sourceType, profileID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, profile)
REDACTED

func (h *DataManagementHandler) ListS3Profiles(c *gin.Context) {
	if !h.requireAgentEnabled(c) {
		return
REDACTED

	items, err := h.dataManagementService.ListS3Profiles(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"items": itemsREDACTED)
REDACTED

func (h *DataManagementHandler) CreateS3Profile(c *gin.Context) {
	var req CreateS3ProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED

	profile, err := h.dataManagementService.CreateS3Profile(c.Request.Context(), service.DataManagementCreateS3ProfileInput{
		ProfileID: req.ProfileID,
		Name:      req.Name,
		SetActive: req.SetActive,
		S3: service.DataManagementS3Config{
			Enabled:         req.Enabled,
			Endpoint:        req.Endpoint,
			Region:          req.Region,
			Bucket:          req.Bucket,
			AccessKeyID:     req.AccessKeyID,
			SecretAccessKey: req.SecretAccessKey,
			Prefix:          req.Prefix,
			ForcePathStyle:  req.ForcePathStyle,
			UseSSL:          req.UseSSL,
	REDACTED,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, profile)
REDACTED

func (h *DataManagementHandler) UpdateS3Profile(c *gin.Context) {
	var req UpdateS3ProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
REDACTED

	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Invalid profile_id")
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED

	profile, err := h.dataManagementService.UpdateS3Profile(c.Request.Context(), service.DataManagementUpdateS3ProfileInput{
		ProfileID: profileID,
		Name:      req.Name,
		S3: service.DataManagementS3Config{
			Enabled:         req.Enabled,
			Endpoint:        req.Endpoint,
			Region:          req.Region,
			Bucket:          req.Bucket,
			AccessKeyID:     req.AccessKeyID,
			SecretAccessKey: req.SecretAccessKey,
			Prefix:          req.Prefix,
			ForcePathStyle:  req.ForcePathStyle,
			UseSSL:          req.UseSSL,
	REDACTED,
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, profile)
REDACTED

func (h *DataManagementHandler) DeleteS3Profile(c *gin.Context) {
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Invalid profile_id")
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	if err := h.dataManagementService.DeleteS3Profile(c.Request.Context(), profileID); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, gin.H{"deleted": trueREDACTED)
REDACTED

func (h *DataManagementHandler) SetActiveS3Profile(c *gin.Context) {
	profileID := strings.TrimSpace(c.Param("profile_id"))
	if profileID == "" {
		response.BadRequest(c, "Invalid profile_id")
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	profile, err := h.dataManagementService.SetActiveS3Profile(c.Request.Context(), profileID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, profile)
REDACTED

func (h *DataManagementHandler) ListBackupJobs(c *gin.Context) {
	if !h.requireAgentEnabled(c) {
		return
REDACTED

	pageSize := int32(20)
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			response.BadRequest(c, "Invalid page_size")
			return
	REDACTED
		pageSize = int32(v)
REDACTED

	result, err := h.dataManagementService.ListBackupJobs(c.Request.Context(), service.DataManagementListBackupJobsInput{
		PageSize:   pageSize,
		PageToken:  c.Query("page_token"),
		Status:     c.Query("status"),
		BackupType: c.Query("backup_type"),
REDACTED)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *DataManagementHandler) GetBackupJob(c *gin.Context) {
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		response.BadRequest(c, "Invalid backup job ID")
		return
REDACTED

	if !h.requireAgentEnabled(c) {
		return
REDACTED
	job, err := h.dataManagementService.GetBackupJob(c.Request.Context(), jobID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, job)
REDACTED

func (h *DataManagementHandler) requireAgentEnabled(c *gin.Context) bool {
	if h.dataManagementService == nil {
		err := infraerrors.ServiceUnavailable(
			service.DataManagementAgentUnavailableReason,
			"data management agent service is not configured",
		).WithMetadata(map[string]string{"socket_path": service.DefaultDataManagementAgentSocketPathREDACTED)
		response.ErrorFrom(c, err)
		return false
REDACTED

	if err := h.dataManagementService.EnsureAgentEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return false
REDACTED

	return true
REDACTED

func (h *DataManagementHandler) getAgentHealth(c *gin.Context) service.DataManagementAgentHealth {
	if h.dataManagementService == nil {
		return service.DataManagementAgentHealth{
			Enabled:    false,
			Reason:     service.DataManagementAgentUnavailableReason,
			SocketPath: service.DefaultDataManagementAgentSocketPath,
	REDACTED
REDACTED
	return h.dataManagementService.GetAgentHealth(c.Request.Context())
REDACTED

func normalizeBackupIdempotencyKey(headerValue, bodyValue string) string {
	headerKey := strings.TrimSpace(headerValue)
	if headerKey != "" {
		return headerKey
REDACTED
	return strings.TrimSpace(bodyValue)
REDACTED
