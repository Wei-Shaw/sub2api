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

func forEachOpenAISSEFrame(body string, fn func(string, []byte)) {
	if fn == nil || strings.TrimSpace(body) == "" {
		return
REDACTED
	var parser openAICompatSSEFrameParser
	emit := func(frame openAICompatSSEFrame, ok bool) {
		if !ok {
			return
	REDACTED
		emitData := func(value string) {
			value = strings.TrimSpace(value)
			if value == "" || value == "[DONE]" {
				return
		REDACTED
			data := []byte(value)
			fn(effectiveOpenAISSEEventType(data, frame.EventType), data)
	REDACTED
		if gjson.Valid(frame.Data) {
			emitData(frame.Data)
			return
	REDACTED
		for _, value := range strings.Split(frame.Data, "\n") {
			emitData(value)
	REDACTED
REDACTED
	for _, line := range strings.Split(body, "\n") {
		emit(parser.AddLine(strings.TrimRight(line, "\r")))
REDACTED
	emit(parser.Finish())
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
