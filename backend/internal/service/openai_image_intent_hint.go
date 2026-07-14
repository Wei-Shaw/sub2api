package service

import "github.com/gin-gonic/gin"

const openAIImageIntentHintContextKey = "openai_image_intent_hint"

type openAIImageIntentClassifier func(endpoint string, requestedModel string, body []byte) bool

// SetOpenAIImageIntentHint records the canonical request body's image intent.
func SetOpenAIImageIntentHint(c *gin.Context, imageIntent bool) {
	if c == nil || GetOpenAIClientTransport(c) != OpenAIClientTransportHTTP {
		return
REDACTED
	c.Set(openAIImageIntentHintContextKey, imageIntent)
REDACTED

func getOpenAIImageIntentHint(c *gin.Context) (imageIntent bool, known bool) {
	if c == nil || GetOpenAIClientTransport(c) != OpenAIClientTransportHTTP {
		return false, false
REDACTED
	value, ok := c.Get(openAIImageIntentHintContextKey)
	if !ok {
		return false, false
REDACTED
	imageIntent, ok = value.(bool)
	return imageIntent, ok
REDACTED

func resolveOpenAIImageIntentHint(
	c *gin.Context,
	requestedModel string,
	canonicalBody []byte,
	classify openAIImageIntentClassifier,
) bool {
	if imageIntent, known := getOpenAIImageIntentHint(c); known {
		return imageIntent
REDACTED
	imageIntent := classify(openAIResponsesEndpoint, requestedModel, canonicalBody)
	SetOpenAIImageIntentHint(c, imageIntent)
	return imageIntent
REDACTED

func resolveOpenAIPassthroughImageIntent(
	c *gin.Context,
	canonicalRequestedModel string,
	canonicalBody []byte,
	attemptRequestedModel string,
	attemptBody []byte,
	attemptInvalidated bool,
	classify openAIImageIntentClassifier,
) bool {
	imageIntent := resolveOpenAIImageIntentHint(c, canonicalRequestedModel, canonicalBody, classify)
	if attemptInvalidated {
		imageIntent = classify(openAIResponsesEndpoint, attemptRequestedModel, attemptBody)
REDACTED
	return imageIntent
REDACTED
