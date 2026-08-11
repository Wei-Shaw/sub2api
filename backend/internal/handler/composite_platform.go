package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func ensureCompositeTargetPlatform(c *gin.Context, apiKey *service.APIKey, model string) {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return
REDACTED
	if _, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
		return
REDACTED
	if platform, ok := service.DetectModelPlatform(model); ok {
		c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), platform))
REDACTED
REDACTED

func compositeTargetPlatformAllowed(c *gin.Context, apiKey *service.APIKey, model string, allowed ...string) bool {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return true
REDACTED
	ensureCompositeTargetPlatform(c, apiKey, model)
	platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	if !ok {
		return false
REDACTED
	for _, allowedPlatform := range allowed {
		if platform == allowedPlatform {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func compositeTargetPlatformResolved(c *gin.Context, apiKey *service.APIKey, model string) bool {
	if c == nil || c.Request == nil || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
		return true
REDACTED
	ensureCompositeTargetPlatform(c, apiKey, model)
	_, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
	return ok
REDACTED

func effectiveAPIKeyPlatform(c *gin.Context, apiKey *service.APIKey) string {
	if c != nil && c.Request != nil {
		if platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
			return platform
	REDACTED
REDACTED
	if apiKey == nil || apiKey.Group == nil {
		return ""
REDACTED
	return apiKey.Group.Platform
REDACTED

func openAIReasoningEffortPolicyForRequest(c *gin.Context, apiKey *service.APIKey) (string, []service.ReasoningEffortMapping, bool) {
	if apiKey == nil || apiKey.Group == nil {
		return "", nil, false
REDACTED
	if apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformComposite {
		return "", nil, false
REDACTED
	if effectiveAPIKeyPlatform(c, apiKey) != service.PlatformOpenAI {
		return "", nil, false
REDACTED
	return apiKey.Group.MaxReasoningEffort, apiKey.Group.ReasoningEffortMappings, true
REDACTED

func applyOpenAIReasoningEffortPolicyForRequest(c *gin.Context, apiKey *service.APIKey, body []byte) ([]byte, bool) {
	maxEffort, mappings, ok := openAIReasoningEffortPolicyForRequest(c, apiKey)
	if !ok {
		return body, false
REDACTED
	return service.ApplyOpenAIReasoningEffortPolicy(body, maxEffort, mappings)
REDACTED

func bindOpenAIReasoningEffortPolicyForMessagesRequest(c *gin.Context, apiKey *service.APIKey, body []byte) {
	if c == nil || c.Request == nil {
		return
REDACTED
	// The Messages bridge synthesizes a default OpenAI effort when
	// output_config.effort is omitted. Bind the group policy only for an
	// explicit client value so the ceiling does not alter that default.
	effort := gjson.GetBytes(body, "output_config.effort")
	if !effort.Exists() || effort.Type != gjson.String || strings.TrimSpace(effort.String()) == "" {
		return
REDACTED
	maxEffort, mappings, ok := openAIReasoningEffortPolicyForRequest(c, apiKey)
	if !ok {
		return
REDACTED
	c.Request = c.Request.WithContext(service.WithOpenAIReasoningEffortPolicy(c.Request.Context(), maxEffort, mappings))
REDACTED
