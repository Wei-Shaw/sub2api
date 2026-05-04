package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsImageGenerationIntent(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		model    string
		body     []byte
		want     bool
REDACTED{
		{
			name:     "images endpoint",
			endpoint: "/v1/images/generations",
			body:     []byte(`{"model":"gpt-image-2"REDACTED`),
			want:     true,
	REDACTED,
		{
			name:     "image model",
			endpoint: "/v1/responses",
			model:    "gpt-image-2",
			body:     []byte(`{"model":"gpt-image-2"REDACTED`),
			want:     true,
	REDACTED,
		{
			name:     "image tool",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"REDACTED]REDACTED`),
			want:     true,
	REDACTED,
		{
			name:     "image tool choice",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","tool_choice":{"type":"image_generation"REDACTEDREDACTED`),
			want:     true,
	REDACTED,
		{
			name:     "required tool choice alone is text",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","tool_choice":"required"REDACTED`),
			want:     false,
	REDACTED,
		{
			name:     "text only gpt 5.4",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","input":"write code"REDACTED`),
			want:     false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsImageGenerationIntent(tt.endpoint, tt.model, tt.body))
	REDACTED)
REDACTED
REDACTED

func TestResolveOpenAIResponsesImageBillingConfigUsesCurrentBodyModel(t *testing.T) {
	imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(
		[]byte(`{"model":"mapped-image-model","tools":[{"type":"image_generation","size":"1024x1024"REDACTED]REDACTED`),
		"requested-model",
	)
REDACTED
	require.Equal(t, "mapped-image-model", imageModel)
	require.Equal(t, "1K", imageSize)
REDACTED

func TestResolveOpenAIResponsesImageBillingConfigToolModelWins(t *testing.T) {
	imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(
		[]byte(`{"model":"mapped-text-model","tools":[{"type":"image_generation","model":"gpt-image-2","size":"1536x1024"REDACTED]REDACTED`),
		"requested-model",
	)
REDACTED
	require.Equal(t, "gpt-image-2", imageModel)
	require.Equal(t, "2K", imageSize)
REDACTED

func TestResolveOpenAIResponsesImageBillingConfigSupportsOfficialAndCustomSizes(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantTier string
REDACTED{
		{
			name:     "official 2k landscape",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"gpt-image-2","size":"2048x1152"REDACTED]REDACTED`),
			wantTier: "2K",
	REDACTED,
		{
			name:     "official 4k landscape",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"gpt-image-2","size":"3840x2160"REDACTED]REDACTED`),
			wantTier: "4K",
	REDACTED,
		{
			name:     "custom valid 2k",
			body:     []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation","model":"gpt-image-2","size":"1280x768"REDACTED]REDACTED`),
			wantTier: "2K",
	REDACTED,
		{
			name:     "default image tool model supports flexible size",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","size":"2048x1152"REDACTED]REDACTED`),
			wantTier: "2K",
	REDACTED,
		{
			name:     "top level image size is moved into billing",
			body:     []byte(`{"model":"gpt-image-2","size":"2048x2048","tools":[{"type":"image_generation","model":"gpt-image-2"REDACTED]REDACTED`),
			wantTier: "2K",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(tt.body, "requested-model")
		REDACTED
			require.NotEmpty(t, imageModel)
			require.Equal(t, tt.wantTier, imageSize)
	REDACTED)
REDACTED
REDACTED

func TestResolveOpenAIResponsesImageBillingConfigDoesNotRejectUnknownSizes(t *testing.T) {
	imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(
		[]byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"gpt-image-1.5","size":"2048x1152"REDACTED]REDACTED`),
		"requested-model",
	)
REDACTED
	require.Equal(t, "gpt-image-1.5", imageModel)
	require.Equal(t, "2K", imageSize)
REDACTED

func TestOpenAIImageOutputCounterDeduplicatesFinalImages(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEData([]byte(`{"type":"response.image_generation_call.partial_image","partial_image_b64":"abc"REDACTED`))
	counter.AddSSEData([]byte(`{"type":"response.output_item.done","item":{"id":"ig_1","type":"image_generation_call","result":"final-a"REDACTEDREDACTED`))
	counter.AddSSEData([]byte(`{"type":"response.completed","response":{"output":[{"id":"ig_1","type":"image_generation_call","result":"final-a"REDACTED,{"id":"ig_2","type":"image_generation_call","result":"final-b"REDACTED]REDACTEDREDACTED`))
	require.Equal(t, 2, counter.Count())
REDACTED

func TestOpenAIImageOutputCounterCountsImagesAPIStreamShapes(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEData([]byte(`{"type":"image_generation.completed","id":"ig_complete","b64_json":"final-a"REDACTED`))
	counter.AddSSEData([]byte(`{"type":"response.output_item.done","item":{"id":"ig_item","type":"image_generation_call","result":"final-b"REDACTEDREDACTED`))
	counter.AddSSEData([]byte(`{"type":"response.completed","response":{"output":[{"id":"ig_done","type":"image_generation_call","result":"final-c"REDACTED]REDACTEDREDACTED`))
	require.Equal(t, 3, counter.Count())

	dataCounter := newOpenAIImageOutputCounter()
	dataCounter.AddSSEData([]byte(`{"data":[{"b64_json":"a"REDACTED,{"b64_json":"b"REDACTED]REDACTED`))
	dataCounter.AddSSEData([]byte(`{"data":[{"b64_json":"a"REDACTED,{"b64_json":"b"REDACTED,{"b64_json":"c"REDACTED]REDACTED`))
	require.Equal(t, 3, dataCounter.Count())
REDACTED

func TestOpenAIImageOutputCounterCountsMultilineSSEDataPayload(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEData([]byte("{\"type\":\"image_generation.completed\",\n\"b64_json\":\"final-a\"REDACTED"))
	require.Equal(t, 1, counter.Count())
REDACTED

func TestOpenAIImageOutputCounterCountsMultilineSSEBodyPayload(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(
		"data: {\"type\":\"image_generation.completed\",\n" +
			"data: \"b64_json\":\"final-a\"REDACTED\n\n" +
			"data: [DONE]\n\n",
	)
	require.Equal(t, 1, counter.Count())
REDACTED

func TestOpenAIImageOutputCounterFallsBackForInvalidMultilineSSEBody(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(
		"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"final-a\"REDACTED\n" +
			"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"final-b\"REDACTED\n\n",
	)
	require.Equal(t, 2, counter.Count())
REDACTED
