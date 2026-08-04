package service

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	upstreamResponseModelObserverContextKey = "upstream_response_model_observer"
	upstreamResponseModelMaxLength          = 200
)

// upstreamResponseModelObserver tracks one forwarding attempt (or one WS turn).
// A terminal declaration wins over an earlier declaration; otherwise the first
// declaration is retained. Conflicts are diagnostic only and never affect the
// forwarding or billing path.
type upstreamResponseModelObserver struct {
	first    string
	terminal string
	conflict bool
REDACTED

func (o *upstreamResponseModelObserver) Observe(model string, terminal bool) {
	model = normalizeObservedUpstreamResponseModel(model)
	if model == "" {
		return
REDACTED
	current := o.Model()
	if current != "" && !strings.EqualFold(current, model) {
		o.conflict = true
REDACTED
	if terminal {
		o.terminal = model
		return
REDACTED
	if o.first == "" {
		o.first = model
REDACTED
REDACTED

func normalizeObservedUpstreamResponseModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
REDACTED
	runes := []rune(model)
	if len(runes) > upstreamResponseModelMaxLength {
		model = string(runes[:upstreamResponseModelMaxLength])
REDACTED
	return model
REDACTED

func (o *upstreamResponseModelObserver) ObserveOpenAI(payload []byte, eventType string) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return
REDACTED
	model := firstTrimmedGJSONModel(
		gjson.GetBytes(payload, "response.model"),
		gjson.GetBytes(payload, "model"),
	)
	o.Observe(model, isUpstreamResponseModelTerminalEvent(eventType))
REDACTED

func (o *upstreamResponseModelObserver) ObserveAnthropic(payload []byte) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return
REDACTED
	model := firstTrimmedGJSONModel(
		gjson.GetBytes(payload, "message.model"),
		gjson.GetBytes(payload, "model"),
	)
	o.Observe(model, false)
REDACTED

func (o *upstreamResponseModelObserver) ObserveGemini(payload []byte) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return
REDACTED
	model := firstTrimmedGJSONModel(
		gjson.GetBytes(payload, "modelVersion"),
		gjson.GetBytes(payload, "response.modelVersion"),
	)
	// Gemini streaming has no universal terminal event carrying modelVersion;
	// treating each declaration as terminal retains the latest chunk.
	o.Observe(model, true)
REDACTED

func (o *upstreamResponseModelObserver) Model() string {
	if o == nil {
		return ""
REDACTED
	if o.terminal != "" {
		return o.terminal
REDACTED
	return o.first
REDACTED

func (o *upstreamResponseModelObserver) Conflict() bool {
	return o != nil && o.conflict
REDACTED

func beginUpstreamResponseModelObservation(c *gin.Context) *upstreamResponseModelObserver {
	observer := &upstreamResponseModelObserver{REDACTED
	if c != nil {
		c.Set(upstreamResponseModelObserverContextKey, observer)
REDACTED
	return observer
REDACTED

func upstreamResponseModelObserverFromContext(c *gin.Context) *upstreamResponseModelObserver {
	if c == nil {
		return nil
REDACTED
	value, ok := c.Get(upstreamResponseModelObserverContextKey)
	if !ok {
		return nil
REDACTED
	observer, _ := value.(*upstreamResponseModelObserver)
	return observer
REDACTED

func observedUpstreamResponseModel(c *gin.Context) string {
	return upstreamResponseModelObserverFromContext(c).Model()
REDACTED

func observedUpstreamResponseModelConflict(c *gin.Context) bool {
	return upstreamResponseModelObserverFromContext(c).Conflict()
REDACTED

func observeOpenAISSEBody(observer *upstreamResponseModelObserver, body string) {
	if observer == nil || strings.TrimSpace(body) == "" {
		return
REDACTED
	forEachOpenAISSEDataPayload(body, func(payload []byte) {
		eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
		observer.ObserveOpenAI(payload, eventType)
REDACTED)
REDACTED

func firstTrimmedGJSONModel(values ...gjson.Result) string {
	for _, value := range values {
		if !value.Exists() || value.Type != gjson.String {
			continue
	REDACTED
		if model := strings.TrimSpace(value.String()); model != "" {
			return model
	REDACTED
REDACTED
	return ""
REDACTED

func isUpstreamResponseModelTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
REDACTED
REDACTED

func upstreamModelMismatch(sentModel, responseModel string) *bool {
	responseModel = strings.TrimSpace(responseModel)
	if responseModel == "" {
		return nil
REDACTED
	sentModel = strings.TrimSpace(sentModel)
	mismatch := sentModel == "" || !strings.EqualFold(sentModel, responseModel)
	return &mismatch
REDACTED

func upstreamSentModel(requestedModel, upstreamModel string) string {
	sentModel := strings.TrimSpace(upstreamModel)
	if sentModel == "" {
		sentModel = strings.TrimSpace(requestedModel)
REDACTED
	return sentModel
REDACTED
