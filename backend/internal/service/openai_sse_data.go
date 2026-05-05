package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

type openAISSEDataAccumulator struct {
	lines []string
REDACTED

func (a *openAISSEDataAccumulator) AddLine(line string, fn func([]byte)) {
	if fn == nil {
		return
REDACTED
	trimmedLine := strings.TrimRight(line, "\r\n")
	if data, ok := extractOpenAISSEDataLine(trimmedLine); ok {
		a.lines = append(a.lines, data)
		return
REDACTED
	if strings.TrimSpace(trimmedLine) == "" {
		a.Flush(fn)
REDACTED
REDACTED

func (a *openAISSEDataAccumulator) Flush(fn func([]byte)) {
	if fn == nil || len(a.lines) == 0 {
		return
REDACTED
	emitOpenAISSEDataPayloads(a.lines, fn)
	a.lines = a.lines[:0]
REDACTED

func forEachOpenAISSEDataPayload(body string, fn func([]byte)) {
	if fn == nil || strings.TrimSpace(body) == "" {
		return
REDACTED
	var acc openAISSEDataAccumulator
	for _, line := range strings.Split(body, "\n") {
		acc.AddLine(line, fn)
REDACTED
	acc.Flush(fn)
REDACTED

func emitOpenAISSEDataPayloads(lines []string, fn func([]byte)) {
	if fn == nil || len(lines) == 0 {
		return
REDACTED
	if len(lines) == 1 {
		emitOpenAISSEDataPayload(lines[0], fn)
		return
REDACTED
	joined := strings.Join(lines, "\n")
	if gjson.Valid(joined) {
		emitOpenAISSEDataPayload(joined, fn)
		return
REDACTED
	for _, line := range lines {
		emitOpenAISSEDataPayload(line, fn)
REDACTED
REDACTED

func emitOpenAISSEDataPayload(data string, fn func([]byte)) {
	data = strings.TrimSpace(data)
	if data == "" || data == "[DONE]" {
		return
REDACTED
	fn([]byte(data))
REDACTED
