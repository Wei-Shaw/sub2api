package service

import (
	"math"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Official Imagine image geometry: https://docs.x.ai/developers/model-capabilities/images/generation
var grokImagineAspectRatioValues = []struct {
	label string
	ratio float64
REDACTED{
	{"1:1", 1REDACTED,
	{"16:9", 16.0 / 9.0REDACTED,
	{"9:16", 9.0 / 16.0REDACTED,
	{"4:3", 4.0 / 3.0REDACTED,
	{"3:4", 3.0 / 4.0REDACTED,
	{"3:2", 1.5REDACTED,
	{"2:3", 2.0 / 3.0REDACTED,
	{"2:1", 2REDACTED,
	{"1:2", 0.5REDACTED,
	{"19.5:9", 19.5 / 9.0REDACTED,
	{"9:19.5", 9.0 / 19.5REDACTED,
	{"20:9", 20.0 / 9.0REDACTED,
	{"9:20", 9.0 / 20.0REDACTED,
REDACTED

func applyGrokImagineImageGeometry(body []byte) ([]byte, error) {
	size := strings.TrimSpace(gjson.GetBytes(body, "size").String())
	resolution := grokImagineImageResolution(gjson.GetBytes(body, "resolution").String())
	aspect := strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String())
	out := append([]byte(nil), body...)

	if resolution == "" {
		if derived := grokImagineImageResolutionFromSize(size); derived != "" {
			next, err := sjson.SetBytes(out, "resolution", derived)
			if err != nil {
				return nil, err
		REDACTED
			out = next
	REDACTED
REDACTED else if gjson.GetBytes(body, "resolution").String() != resolution {
		next, err := sjson.SetBytes(out, "resolution", resolution)
		if err != nil {
			return nil, err
	REDACTED
		out = next
REDACTED

	if aspect == "" {
		if derived := grokImagineAspectRatioFromSize(size); derived != "" {
			next, err := sjson.SetBytes(out, "aspect_ratio", derived)
			if err != nil {
				return nil, err
		REDACTED
			out = next
	REDACTED
REDACTED

	if !gjson.GetBytes(out, "size").Exists() {
		return out, nil
REDACTED
	return sjson.DeleteBytes(out, "size")
REDACTED

func assignGrokMediaResolution(value string, info *GrokMediaRequestInfo) {
	if info == nil {
		return
REDACTED
	value = strings.TrimSpace(value)
	if value == "" {
		return
REDACTED
	if img := grokImagineImageResolution(value); img != "" {
		info.ImageResolution = img
		return
REDACTED
	info.Resolution = value
REDACTED

func grokImagineImageResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1k":
		return "1k"
	case "2k":
		return "2k"
	default:
		return ""
REDACTED
REDACTED

func grokImagineImageResolutionFromSize(size string) string {
	if explicit := grokImagineImageResolution(size); explicit != "" {
		return explicit
REDACTED
	tier, ok := ClassifyImageBillingTier(size)
	if !ok {
		return ""
REDACTED
	if tier == ImageBillingSize1K {
		return "1k"
REDACTED
	return "2k"
REDACTED

func grokImagineAspectRatioFromSize(size string) string {
	width, height, ok := parseImageBillingDimensions(strings.TrimSpace(size))
	if !ok || width <= 0 || height <= 0 {
		return ""
REDACTED
	div := grokImagineGCD(width, height)
	exact := strconv.Itoa(width/div) + ":" + strconv.Itoa(height/div)
	for _, candidate := range grokImagineAspectRatioValues {
		if candidate.label == exact {
			return exact
	REDACTED
REDACTED
	ratio := float64(width) / float64(height)
	bestLabel := ""
	bestDelta := math.MaxFloat64
	for _, candidate := range grokImagineAspectRatioValues {
		delta := math.Abs(ratio - candidate.ratio)
		if delta < bestDelta {
			bestDelta = delta
			bestLabel = candidate.label
	REDACTED
REDACTED
	return bestLabel
REDACTED

func grokImagineGCD(a, b int) int {
	if a < 0 {
		a = -a
REDACTED
	if b < 0 {
		b = -b
REDACTED
	for b != 0 {
		a, b = b, a%b
REDACTED
	if a == 0 {
		return 1
REDACTED
	return a
REDACTED
