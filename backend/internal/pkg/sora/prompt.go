package sora

import (
	"regexp"
	"strings"
)

var storyboardRe = regexp.MustCompile(`\[(\d+(?:\.\d+)?)s\]`)

// IsStoryboardPrompt 检测是否为分镜提示词。
func IsStoryboardPrompt(prompt string) bool {
	if strings.TrimSpace(prompt) == "" {
		return false
REDACTED
	return storyboardRe.MatchString(prompt)
REDACTED

// FormatStoryboardPrompt 将分镜提示词转换为 API 需要的格式。
func FormatStoryboardPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return prompt
REDACTED
	matches := storyboardRe.FindAllStringSubmatchIndex(prompt, -1)
	if len(matches) == 0 {
		return prompt
REDACTED
	firstIdx := matches[0][0]
	instructions := strings.TrimSpace(prompt[:firstIdx])

	shotPattern := regexp.MustCompile(`\[(\d+(?:\.\d+)?)s\]\s*([^\[]+)`)
	shotMatches := shotPattern.FindAllStringSubmatch(prompt, -1)
	if len(shotMatches) == 0 {
		return prompt
REDACTED

	shots := make([]string, 0, len(shotMatches))
	for i, sm := range shotMatches {
		if len(sm) < 3 {
			continue
	REDACTED
		duration := strings.TrimSpace(sm[1])
		scene := strings.TrimSpace(sm[2])
		shots = append(shots, "Shot "+itoa(i+1)+":\nduration: "+duration+"sec\nScene: "+scene)
REDACTED

	timeline := strings.Join(shots, "\n\n")
	if instructions != "" {
		return "current timeline:\n" + timeline + "\n\ninstructions:\n" + instructions
REDACTED
	return timeline
REDACTED

// ExtractRemixID 提取分享链接中的 remix ID。
func ExtractRemixID(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
REDACTED
	re := regexp.MustCompile(`s_[a-f0-9]{32REDACTED`)
	match := re.FindString(text)
	return match
REDACTED
