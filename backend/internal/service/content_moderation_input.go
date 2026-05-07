package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/tidwall/gjson"
)

func ExtractContentModerationText(protocol string, body []byte) string {
	return ExtractContentModerationInput(protocol, body).Text
REDACTED

func ExtractContentModerationInput(protocol string, body []byte) ContentModerationInput {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ContentModerationInput{REDACTED
REDACTED
	var parts []string
	var images []string
	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		collectLastAnthropicUserMessage(gjson.GetBytes(body, "messages"), &parts, &images)
	case ContentModerationProtocolOpenAIChat:
		collectLastRoleMessage(gjson.GetBytes(body, "messages"), "user", &parts, &images)
	case ContentModerationProtocolOpenAIResponses:
		collectLastResponsesInput(gjson.GetBytes(body, "input"), &parts, &images)
	case ContentModerationProtocolGemini:
		collectLastGeminiContent(gjson.GetBytes(body, "contents"), &parts, &images)
	case ContentModerationProtocolOpenAIImages:
		addModerationText(&parts, gjson.GetBytes(body, "prompt").String())
		collectContentValue(gjson.GetBytes(body, "images"), &parts, &images)
	default:
		collectLastResponsesInput(gjson.GetBytes(body, "input"), &parts, &images)
		collectLastRoleMessage(gjson.GetBytes(body, "messages"), "user", &parts, &images)
		collectLastGeminiContent(gjson.GetBytes(body, "contents"), &parts, &images)
REDACTED
	out := ContentModerationInput{
		Text:   normalizeContentModerationText(strings.Join(parts, "\n")),
		Images: normalizeModerationImages(images),
REDACTED
	out.Normalize()
	return out
REDACTED

func collectLastRoleMessage(messages gjson.Result, role string, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
REDACTED
	var lastParts []string
	var lastImages []string
	messages.ForEach(func(_, msg gjson.Result) bool {
		if strings.ToLower(strings.TrimSpace(msg.Get("role").String())) == role {
			var candidate []string
			var candidateImages []string
			collectContentValue(msg.Get("content"), &candidate, &candidateImages)
			if normalizeContentModerationText(strings.Join(candidate, "\n")) != "" || len(candidateImages) > 0 {
				lastParts = candidate
				lastImages = candidateImages
		REDACTED
	REDACTED
		return true
REDACTED)
	*parts = append(*parts, lastParts...)
	*images = append(*images, lastImages...)
REDACTED

func collectLastAnthropicUserMessage(messages gjson.Result, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
REDACTED
	var lastParts []string
	var lastImages []string
	messages.ForEach(func(_, msg gjson.Result) bool {
		if strings.ToLower(strings.TrimSpace(msg.Get("role").String())) == "user" {
			var candidate []string
			var candidateImages []string
			collectAnthropicUserContentValue(msg.Get("content"), &candidate, &candidateImages)
			if normalizeContentModerationText(strings.Join(candidate, "\n")) != "" || len(candidateImages) > 0 {
				lastParts = candidate
				lastImages = candidateImages
		REDACTED
	REDACTED
		return true
REDACTED)
	*parts = append(*parts, lastParts...)
	*images = append(*images, lastImages...)
REDACTED

func collectAnthropicUserContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		if !isAnthropicSystemReminderText(value.String()) {
			addModerationText(parts, value.String())
	REDACTED
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectAnthropicUserContentValue(item, parts, images)
			return true
	REDACTED)
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() && !isAnthropicSystemReminderText(value.Get("text").String()) {
				addModerationText(parts, value.Get("text").String())
		REDACTED
			if value.Get("content").Exists() {
				collectAnthropicUserContentValue(value.Get("content"), parts, images)
		REDACTED
		case "image_url", "input_image", "image":
			collectContentValue(value, parts, images)
	REDACTED
REDACTED
REDACTED

func isAnthropicSystemReminderText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "<system-reminder>")
REDACTED

func collectLastResponsesInput(input gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !input.Exists():
		return
	case input.Type == gjson.String:
		addModerationText(parts, input.String())
	case input.IsArray():
		var last gjson.Result
		input.ForEach(func(_, item gjson.Result) bool {
			if isResponsesUserTextItem(item) {
				last = item
		REDACTED
			return true
	REDACTED)
		if last.Exists() {
			collectContentValue(last.Get("content"), parts, images)
			if last.Get("type").String() == "input_text" || last.Get("text").Exists() {
				collectContentValue(last, parts, images)
		REDACTED
	REDACTED
	case input.IsObject():
		if isResponsesUserTextItem(input) {
			collectContentValue(input.Get("content"), parts, images)
			if input.Get("type").String() == "input_text" || input.Get("text").Exists() {
				collectContentValue(input, parts, images)
		REDACTED
	REDACTED
REDACTED
REDACTED

func isResponsesUserTextItem(item gjson.Result) bool {
	role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
	if role == "user" {
		return responseItemHasModerationText(item)
REDACTED
	if role != "" {
		return false
REDACTED
	return responseItemHasModerationText(item)
REDACTED

func responseItemHasModerationText(item gjson.Result) bool {
	var parts []string
	var images []string
	collectContentValue(item.Get("content"), &parts, &images)
	if item.Get("type").String() == "input_text" || item.Get("text").Exists() {
		collectContentValue(item, &parts, &images)
REDACTED
	return normalizeContentModerationText(strings.Join(parts, "\n")) != "" || len(images) > 0
REDACTED

func collectLastGeminiContent(contents gjson.Result, parts *[]string, images *[]string) {
	if !contents.IsArray() {
		return
REDACTED
	var lastParts []string
	var lastImages []string
	contents.ForEach(func(_, content gjson.Result) bool {
		role := strings.ToLower(strings.TrimSpace(content.Get("role").String()))
		if role == "" || role == "user" {
			var candidate []string
			var candidateImages []string
			if arr := content.Get("parts"); arr.IsArray() {
				arr.ForEach(func(_, part gjson.Result) bool {
					addModerationText(&candidate, part.Get("text").String())
					addGeminiModerationImage(&candidateImages, part)
					return true
			REDACTED)
		REDACTED
			if normalizeContentModerationText(strings.Join(candidate, "\n")) != "" || len(candidateImages) > 0 {
				lastParts = candidate
				lastImages = candidateImages
		REDACTED
	REDACTED
		return true
REDACTED)
	*parts = append(*parts, lastParts...)
	*images = append(*images, lastImages...)
REDACTED

func collectContentValue(value gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !value.Exists():
		return
	case value.Type == gjson.String:
		addModerationText(parts, value.String())
	case value.IsArray():
		value.ForEach(func(_, item gjson.Result) bool {
			collectContentValue(item, parts, images)
			return true
	REDACTED)
	case value.IsObject():
		typ := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		addModerationImage(images, value.Get("image_url.url").String())
		addModerationImage(images, value.Get("image_url").String())
		addModerationImage(images, value.Get("url").String())
		addModerationImageData(images, value.Get("source.media_type").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("source.mediaType").String(), value.Get("source.data").String())
		addModerationImageData(images, value.Get("media_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mime_type").String(), value.Get("data").String())
		addModerationImageData(images, value.Get("mimeType").String(), value.Get("data").String())
		addModerationImage(images, value.Get("source.data").String())
		addModerationImage(images, value.Get("data").String())
		addModerationImage(images, value.Get("base64").String())
		switch typ {
		case "", "text", "input_text", "message":
			if value.Get("text").Exists() {
				addModerationText(parts, value.Get("text").String())
		REDACTED
			if value.Get("content").Exists() {
				collectContentValue(value.Get("content"), parts, images)
		REDACTED
		case "image_url", "input_image", "image":
	REDACTED
REDACTED
REDACTED

func addGeminiModerationImage(images *[]string, part gjson.Result) {
	if inlineData := part.Get("inline_data"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mime_type").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
	REDACTED
REDACTED
	if inlineData := part.Get("inlineData"); inlineData.IsObject() {
		mimeType := strings.TrimSpace(inlineData.Get("mimeType").String())
		data := strings.TrimSpace(inlineData.Get("data").String())
		if mimeType != "" && data != "" {
			addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
	REDACTED
REDACTED
	addModerationImage(images, part.Get("file_data.file_uri").String())
	addModerationImage(images, part.Get("fileData.fileUri").String())
REDACTED

func addModerationImageData(images *[]string, mimeType string, data string) {
	mimeType = strings.TrimSpace(mimeType)
	data = strings.TrimSpace(data)
	if mimeType == "" || data == "" {
		return
REDACTED
	addModerationImage(images, fmt.Sprintf("data:%s;base64,%s", mimeType, data))
REDACTED

func addModerationImage(images *[]string, image string) {
	image = strings.TrimSpace(image)
	if image == "" {
		return
REDACTED
	if strings.HasPrefix(image, "data:") || strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		*images = append(*images, image)
REDACTED
REDACTED

func normalizeModerationImages(images []string) []string {
	out := make([]string, 0, len(images))
	seen := make(map[string]struct{REDACTED, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
	REDACTED
		if _, ok := seen[image]; ok {
			continue
	REDACTED
		seen[image] = struct{REDACTED{REDACTED
		out = append(out, image)
REDACTED
	return out
REDACTED

func limitContentModerationImages(images []string) []string {
	if len(images) <= maxContentModerationInputImages {
		return images
REDACTED
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(images))))
	if err != nil {
		return images[:maxContentModerationInputImages]
REDACTED
	return []string{images[int(idx.Int64())]REDACTED
REDACTED

func addModerationText(parts *[]string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
REDACTED
	if strings.Contains(text, "<system-reminder>") {
		return
REDACTED
	*parts = append(*parts, text)
REDACTED

func normalizeContentModerationText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
REDACTED
