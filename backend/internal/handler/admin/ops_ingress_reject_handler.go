package admin

import (
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

var ingressRejectReasons = map[string]struct{REDACTED{
	"query_api_key_deprecated": {REDACTED, "api_key_required": {REDACTED, "invalid_api_key": {REDACTED,
	"invalid_auth_rate_limited": {REDACTED,
	"api_key_auth_overloaded":   {REDACTED,
	"api_key_disabled":          {REDACTED, "ip_restricted": {REDACTED, "user_inactive": {REDACTED, "group_deleted": {REDACTED,
	"group_disabled": {REDACTED, "group_not_allowed": {REDACTED, "group_unassigned": {REDACTED, "other": {REDACTED,
REDACTED

var ingressRejectRouteFamilies = map[string]struct{REDACTED{
	"antigravity": {REDACTED, "gemini": {REDACTED, "codex": {REDACTED, "messages": {REDACTED, "responses": {REDACTED,
	"chat_completions": {REDACTED, "images": {REDACTED, "videos": {REDACTED, "embeddings": {REDACTED, "models": {REDACTED, "other": {REDACTED,
REDACTED

var ingressRejectProtocols = map[string]struct{REDACTED{
	"google": {REDACTED, "anthropic": {REDACTED, "openai": {REDACTED, "gateway": {REDACTED, "other": {REDACTED,
REDACTED

// ListIngressRejects returns bounded security aggregates, never raw credentials or request bodies.
func (h *OpsHandler) ListIngressRejects(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
REDACTED
	startTime, endTime, err := parseOpsTimeRange(c, "1h")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED
	filter := &service.OpsIngressRejectFilter{Page: page, PageSize: pageSizeREDACTED
	if !startTime.IsZero() {
		filter.StartTime = &startTime
REDACTED
	if !endTime.IsZero() {
		filter.EndTime = &endTime
REDACTED
	if filter.RejectReason, err = parseIngressRejectEnum(c, "reason", ingressRejectReasons); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED
	if filter.RouteFamily, err = parseIngressRejectEnum(c, "route_family", ingressRejectRouteFamilies); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED
	if filter.Protocol, err = parseIngressRejectEnum(c, "protocol", ingressRejectProtocols); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED
	if raw := strings.TrimSpace(c.Query("client_ip")); raw != "" {
		addr, parseErr := netip.ParseAddr(raw)
		if parseErr != nil {
			response.BadRequest(c, "Invalid client_ip")
			return
	REDACTED
		addr = addr.Unmap()
		if addr.Is6() {
			addr = netip.PrefixFrom(addr, 64).Masked().Addr()
	REDACTED
		filter.ClientIP = addr.String()
REDACTED
	if filter.UserID, err = parseOptionalPositiveID(c, "user_id"); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED
	if filter.APIKeyID, err = parseOptionalPositiveID(c, "api_key_id"); err != nil {
		response.BadRequest(c, err.Error())
		return
REDACTED

	result, err := h.opsService.ListIngressRejects(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, result)
REDACTED

func (h *OpsHandler) GetIngressRejectHealth(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
REDACTED
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
REDACTED
	response.Success(c, h.opsService.GetIngressRejectHealth())
REDACTED

func parseIngressRejectEnum(c *gin.Context, name string, allowed map[string]struct{REDACTED) (string, error) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return "", nil
REDACTED
	if _, ok := allowed[value]; !ok {
		return "", &ingressRejectQueryError{message: "Invalid " + nameREDACTED
REDACTED
	return value, nil
REDACTED

func parseOptionalPositiveID(c *gin.Context, name string) (*int64, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
REDACTED
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, &ingressRejectQueryError{message: "Invalid " + nameREDACTED
REDACTED
	return &value, nil
REDACTED

type ingressRejectQueryError struct{ message string REDACTED

func (e *ingressRejectQueryError) Error() string { return e.message REDACTED
