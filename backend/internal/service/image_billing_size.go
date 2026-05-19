package service

import (
	"sort"
	"strconv"
	"strings"
)

const (
	ImageBillingSize1K = "1K"
	ImageBillingSize2K = "2K"
	ImageBillingSize4K = "4K"

	ImageSizeSourceOutput  = "output"
	ImageSizeSourceInput   = "input"
	ImageSizeSourceDefault = "default"
	ImageSizeSourceLegacy  = "legacy"
)

type ImageBillingSizeResolution struct {
	BillingSize string
	InputSize   string
	OutputSize  string
	Source      string
	Breakdown   map[string]int
REDACTED

func ClassifyImageBillingTier(size string) (string, bool) {
	trimmed := strings.TrimSpace(size)
	normalized := strings.ToLower(trimmed)
	switch normalized {
	case "", "auto":
		return "", false
	case "1k":
		return ImageBillingSize1K, true
	case "2k":
		return ImageBillingSize2K, true
	case "4k":
		return ImageBillingSize4K, true
	case "2048x2048", "2048x1152":
		return ImageBillingSize2K, true
	case "3840x2160", "2160x3840":
		return ImageBillingSize4K, true
REDACTED

	width, height, ok := parseImageBillingDimensions(trimmed)
	if !ok {
		return "", false
REDACTED
	maxEdge := width
	if height > maxEdge {
		maxEdge = height
REDACTED
	switch {
	case maxEdge <= 1024:
		return ImageBillingSize1K, true
	case maxEdge <= 2048:
		return ImageBillingSize2K, true
	default:
		return ImageBillingSize4K, true
REDACTED
REDACTED

func NormalizeImageBillingTierOrDefault(size string) string {
	if tier, ok := ClassifyImageBillingTier(size); ok {
		return tier
REDACTED
	return ImageBillingSize2K
REDACTED

func ResolveImageBillingSize(inputSize string, outputSizes []string) ImageBillingSizeResolution {
	inputSize = strings.TrimSpace(inputSize)
	outputSizes = compactTrimmedStrings(outputSizes)

	breakdown := map[string]int{REDACTED
	outputSize := firstDisplayImageOutputSize(outputSizes)
	outputTier := ""
	for _, output := range outputSizes {
		tier, ok := ClassifyImageBillingTier(output)
		if !ok {
			continue
	REDACTED
		breakdown[tier]++
		if imageTierRank(tier) > imageTierRank(outputTier) {
			outputTier = tier
	REDACTED
REDACTED
	if outputTier != "" {
		return ImageBillingSizeResolution{
			BillingSize: outputTier,
			InputSize:   inputSize,
			OutputSize:  outputSize,
			Source:      ImageSizeSourceOutput,
			Breakdown:   normalizeImageSizeBreakdown(breakdown),
	REDACTED
REDACTED

	if tier, ok := ClassifyImageBillingTier(inputSize); ok {
		return ImageBillingSizeResolution{
			BillingSize: tier,
			InputSize:   inputSize,
			OutputSize:  outputSize,
			Source:      ImageSizeSourceInput,
	REDACTED
REDACTED

	return ImageBillingSizeResolution{
		BillingSize: ImageBillingSize2K,
		InputSize:   inputSize,
		OutputSize:  outputSize,
		Source:      ImageSizeSourceDefault,
REDACTED
REDACTED

func ApplyOpenAIImageBillingResolution(result *OpenAIForwardResult) {
	if result == nil || result.ImageCount <= 0 {
		return
REDACTED
	inputSize := strings.TrimSpace(result.ImageInputSize)
	if inputSize == "" && strings.TrimSpace(result.ImageSize) != ImageBillingSize2K {
		inputSize = strings.TrimSpace(result.ImageSize)
REDACTED
	outputSizes := result.ImageOutputSizes
	if len(outputSizes) == 0 && strings.TrimSpace(result.ImageOutputSize) != "" {
		outputSizes = []string{result.ImageOutputSizeREDACTED
REDACTED
	resolved := ResolveImageBillingSize(inputSize, outputSizes)
	applyImageBillingResolution(
		&result.ImageSize,
		&result.ImageInputSize,
		&result.ImageOutputSize,
		&result.ImageSizeSource,
		&result.ImageSizeBreakdown,
		resolved,
	)
REDACTED

func ApplyForwardImageBillingResolution(result *ForwardResult) {
	if result == nil || result.ImageCount <= 0 {
		return
REDACTED
	inputSize := strings.TrimSpace(result.ImageInputSize)
	if inputSize == "" && strings.TrimSpace(result.ImageSize) != ImageBillingSize2K {
		inputSize = strings.TrimSpace(result.ImageSize)
REDACTED
	outputSizes := result.ImageOutputSizes
	if len(outputSizes) == 0 && strings.TrimSpace(result.ImageOutputSize) != "" {
		outputSizes = []string{result.ImageOutputSizeREDACTED
REDACTED
	resolved := ResolveImageBillingSize(inputSize, outputSizes)
	applyImageBillingResolution(
		&result.ImageSize,
		&result.ImageInputSize,
		&result.ImageOutputSize,
		&result.ImageSizeSource,
		&result.ImageSizeBreakdown,
		resolved,
	)
REDACTED

func applyImageBillingResolution(
	billingSize *string,
	inputSize *string,
	outputSize *string,
	source *string,
	breakdown *map[string]int,
	resolved ImageBillingSizeResolution,
) {
	*billingSize = resolved.BillingSize
	*inputSize = resolved.InputSize
	*outputSize = resolved.OutputSize
	*source = resolved.Source
	*breakdown = resolved.Breakdown
REDACTED

func parseImageBillingDimensions(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
REDACTED
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
REDACTED
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
REDACTED
	if width <= 0 || height <= 0 {
		return 0, 0, false
REDACTED
	return width, height, true
REDACTED

func compactTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
REDACTED
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
	REDACTED
REDACTED
	return out
REDACTED

func firstDisplayImageOutputSize(outputSizes []string) string {
	for _, output := range outputSizes {
		if trimmed := strings.TrimSpace(output); trimmed != "" {
			return trimmed
	REDACTED
REDACTED
	return ""
REDACTED

func imageTierRank(tier string) int {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case ImageBillingSize1K:
		return 1
	case ImageBillingSize2K:
		return 2
	case ImageBillingSize4K:
		return 3
	default:
		return 0
REDACTED
REDACTED

func normalizeImageSizeBreakdown(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
REDACTED
	out := make(map[string]int, len(in))
	for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4KREDACTED {
		if count := in[tier]; count > 0 {
			out[tier] = count
	REDACTED
REDACTED
	if len(out) == 0 {
		return nil
REDACTED
	return out
REDACTED

func SortedImageBillingBreakdownKeys(breakdown map[string]int) []string {
	keys := make([]string, 0, len(breakdown))
	for key := range breakdown {
		keys = append(keys, key)
REDACTED
	sort.Slice(keys, func(i, j int) bool {
		left, right := imageTierRank(keys[i]), imageTierRank(keys[j])
		if left == right {
			return keys[i] < keys[j]
	REDACTED
		return left < right
REDACTED)
	return keys
REDACTED
