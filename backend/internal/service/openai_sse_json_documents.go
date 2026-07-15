package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

const (
	maxOpenAIConcatenatedJSONDocuments = 16
	maxOpenAIConcatenatedJSONBytes     = 16 * 1024 * 1024
)

// splitOpenAIConcatenatedJSONDocuments recognizes the narrow corruption shape
// produced when multiple complete Responses events arrive in one transport
// message. Other malformed payloads are left untouched for normal error paths.
func splitOpenAIConcatenatedJSONDocuments(payload []byte) ([][]byte, bool) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || len(payload) > maxOpenAIConcatenatedJSONBytes || json.Valid(payload) {
		return nil, false
REDACTED

	decoder := json.NewDecoder(bytes.NewReader(payload))
	documents := make([][]byte, 0, 2)
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err != nil {
			if err == io.EOF && len(documents) > 1 {
				return documents, true
		REDACTED
			return nil, false
	REDACTED
		raw = bytes.TrimSpace(raw)
		var envelope struct {
			Type string `json:"type"`
	REDACTED
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, false
	REDACTED
		eventType := strings.TrimSpace(envelope.Type)
		if eventType == "" || strings.ContainsAny(eventType, "\r\n") {
			return nil, false
	REDACTED
		if len(documents) == maxOpenAIConcatenatedJSONDocuments {
			return nil, false
	REDACTED
		documents = append(documents, raw)
REDACTED
REDACTED

type openAISSEJSONDocumentScanner struct {
	scanner *bufio.Scanner
	pending []string
	current string
REDACTED

func newOpenAISSEJSONDocumentScanner(scanner *bufio.Scanner) *openAISSEJSONDocumentScanner {
	return &openAISSEJSONDocumentScanner{scanner: scannerREDACTED
REDACTED

func (s *openAISSEJSONDocumentScanner) Scan() bool {
	if len(s.pending) > 0 {
		s.current = s.pending[0]
		s.pending = s.pending[1:]
		return true
REDACTED
	if s.scanner == nil || !s.scanner.Scan() {
		return false
REDACTED

	line := s.scanner.Text()
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		s.current = line
		return true
REDACTED
	if len(data) > maxOpenAIConcatenatedJSONBytes {
		s.current = line
		return true
REDACTED
	documents, repaired := splitOpenAIConcatenatedJSONDocuments([]byte(data))
	if !repaired {
		s.current = line
		return true
REDACTED

	expanded := make([]string, 0, len(documents)*3)
	for i, document := range documents {
		if i > 0 {
			var envelope struct {
				Type string `json:"type"`
		REDACTED
			_ = json.Unmarshal(document, &envelope)
			expanded = append(expanded, "event: "+strings.TrimSpace(envelope.Type))
	REDACTED
		expanded = append(expanded, "data: "+string(document), "")
REDACTED
	s.current = expanded[0]
	s.pending = expanded[1:]
	return true
REDACTED

func (s *openAISSEJSONDocumentScanner) Text() string {
	return s.current
REDACTED

func (s *openAISSEJSONDocumentScanner) Err() error {
	if s.scanner == nil {
		return nil
REDACTED
	return s.scanner.Err()
REDACTED
