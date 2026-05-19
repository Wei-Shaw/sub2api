package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyImageBillingTier(t *testing.T) {
	tests := []struct {
		name     string
		size     string
		wantTier string
		wantOK   bool
REDACTED{
		{name: "explicit 2k square", size: "2048x2048", wantTier: "2K", wantOK: trueREDACTED,
		{name: "explicit 2k landscape", size: "2048x1152", wantTier: "2K", wantOK: trueREDACTED,
		{name: "explicit 4k landscape", size: "3840x2160", wantTier: "4K", wantOK: trueREDACTED,
		{name: "explicit 4k portrait", size: "2160x3840", wantTier: "4K", wantOK: trueREDACTED,
		{name: "long edge 1k", size: "1024X768", wantTier: "1K", wantOK: trueREDACTED,
		{name: "long edge 2k", size: "1280x768", wantTier: "2K", wantOK: trueREDACTED,
		{name: "long edge 4k", size: "2560x1600", wantTier: "4K", wantOK: trueREDACTED,
		{name: "tier string 1k", size: "1k", wantTier: "1K", wantOK: trueREDACTED,
		{name: "empty", size: "", wantOK: falseREDACTED,
		{name: "auto", size: "auto", wantOK: falseREDACTED,
		{name: "invalid", size: "not-a-size", wantOK: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTier, gotOK := ClassifyImageBillingTier(tt.size)
			require.Equal(t, tt.wantOK, gotOK)
			require.Equal(t, tt.wantTier, gotTier)
	REDACTED)
REDACTED
REDACTED

func TestResolveImageBillingSize(t *testing.T) {
	tests := []struct {
		name          string
		inputSize     string
		outputSizes   []string
		wantBilling   string
		wantOutput    string
		wantSource    string
		wantBreakdown map[string]int
REDACTED{
		{
			name:          "output wins over input",
			inputSize:     "1024x1024",
			outputSizes:   []string{"3840x2160"REDACTED,
			wantBilling:   "4K",
			wantOutput:    "3840x2160",
			wantSource:    ImageSizeSourceOutput,
			wantBreakdown: map[string]int{"4K": 1REDACTED,
	REDACTED,
		{
			name:        "input fallback",
			inputSize:   "1024x1024",
			wantBilling: "1K",
			wantSource:  ImageSizeSourceInput,
	REDACTED,
		{
			name:        "auto defaults",
			inputSize:   "auto",
			wantBilling: "2K",
			wantSource:  ImageSizeSourceDefault,
	REDACTED,
		{
			name:        "empty defaults",
			inputSize:   "",
			wantBilling: "2K",
			wantSource:  ImageSizeSourceDefault,
	REDACTED,
		{
			name:        "invalid defaults",
			inputSize:   "largest",
			wantBilling: "2K",
			wantSource:  ImageSizeSourceDefault,
	REDACTED,
		{
			name:          "mixed output chooses highest tier",
			inputSize:     "1024x1024",
			outputSizes:   []string{"1024x1024", "3840x2160", "1280x720"REDACTED,
			wantBilling:   "4K",
			wantOutput:    "1024x1024",
			wantSource:    ImageSizeSourceOutput,
			wantBreakdown: map[string]int{"1K": 1, "2K": 1, "4K": 1REDACTED,
	REDACTED,
		{
			name:        "unparseable output falls back to parseable input",
			inputSize:   "2048x1152",
			outputSizes: []string{"auto"REDACTED,
			wantBilling: "2K",
			wantOutput:  "auto",
			wantSource:  ImageSizeSourceInput,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveImageBillingSize(tt.inputSize, tt.outputSizes)
			require.Equal(t, tt.wantBilling, got.BillingSize)
			require.Equal(t, tt.inputSize, got.InputSize)
			require.Equal(t, tt.wantOutput, got.OutputSize)
			require.Equal(t, tt.wantSource, got.Source)
			require.Equal(t, tt.wantBreakdown, got.Breakdown)
	REDACTED)
REDACTED
REDACTED
