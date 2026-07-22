package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const grokResponsesClientToolMappingContextKey = "grok_responses_client_tool_mapping"

func adaptGrokResponsesClientTools(body []byte) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return body, apicompat.ResponsesClientToolMapping{REDACTED, fmt.Errorf("decode Grok Responses client tools: %w", err)
REDACTED

	mapping, changed, err := apicompat.AdaptResponsesClientTools(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{REDACTED, err
REDACTED
	if !changed {
		return body, mapping, nil
REDACTED
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{REDACTED, fmt.Errorf("encode Grok Responses client tools: %w", err)
REDACTED
	return rebuilt, mapping, nil
REDACTED

func hasGrokResponsesClientToolMapping(mapping apicompat.ResponsesClientToolMapping) bool {
	return len(mapping.CustomTools) > 0 || mapping.ToolSearch || len(mapping.NamespaceTools) > 0
REDACTED

func setGrokResponsesClientToolMapping(c *gin.Context, mapping apicompat.ResponsesClientToolMapping) {
	if c == nil {
		return
REDACTED
	if !hasGrokResponsesClientToolMapping(mapping) {
		clearGrokResponsesClientToolMapping(c)
		return
REDACTED
	c.Set(grokResponsesClientToolMappingContextKey, mapping)
REDACTED

func clearGrokResponsesClientToolMapping(c *gin.Context) {
	if c == nil {
		return
REDACTED
	if _, exists := c.Get(grokResponsesClientToolMappingContextKey); !exists {
		return
REDACTED
	c.Set(grokResponsesClientToolMappingContextKey, apicompat.ResponsesClientToolMapping{REDACTED)
REDACTED

func grokResponsesClientToolMapping(c *gin.Context) (apicompat.ResponsesClientToolMapping, bool) {
	if c == nil {
		return apicompat.ResponsesClientToolMapping{REDACTED, false
REDACTED
	value, ok := c.Get(grokResponsesClientToolMappingContextKey)
	if !ok {
		return apicompat.ResponsesClientToolMapping{REDACTED, false
REDACTED
	mapping, ok := value.(apicompat.ResponsesClientToolMapping)
	return mapping, ok && hasGrokResponsesClientToolMapping(mapping)
REDACTED

func restoreGrokResponsesClientToolPayload(c *gin.Context, payload []byte) ([]byte, error) {
	mapping, ok := grokResponsesClientToolMapping(c)
	if !ok || !bytes.Contains(payload, []byte(`"function_call"`)) || !json.Valid(payload) {
		return payload, nil
REDACTED
	restored, _, err := apicompat.RestoreResponsesClientToolPayload(payload, mapping)
	return restored, err
REDACTED

type grokResponsesClientToolStreamBody struct {
	*io.PipeReader
	source io.Closer
REDACTED

func (b *grokResponsesClientToolStreamBody) Close() error {
	readerErr := b.PipeReader.Close()
	sourceErr := b.source.Close()
	if readerErr != nil {
		return readerErr
REDACTED
	return sourceErr
REDACTED

func newGrokResponsesClientToolStreamBody(
	source io.ReadCloser,
	mapping apicompat.ResponsesClientToolMapping,
	maxLineSize int,
) io.ReadCloser {
	reader, writer := io.Pipe()
	body := &grokResponsesClientToolStreamBody{PipeReader: reader, source: sourceREDACTED
	go transformGrokResponsesClientToolStream(source, writer, mapping, maxLineSize)
	return body
REDACTED

func transformGrokResponsesClientToolStream(
	source io.ReadCloser,
	destination *io.PipeWriter,
	mapping apicompat.ResponsesClientToolMapping,
	maxLineSize int,
) {
	defer func() { _ = source.Close() REDACTED()
	if maxLineSize <= 0 {
		maxLineSize = defaultMaxLineSize
REDACTED

	scanner := bufio.NewScanner(source)
	scanBuf := getSSEScannerBuf64K()
	defer putSSEScannerBuf64K(scanBuf)
	scanner.Buffer(scanBuf[:0], maxLineSize)
	documents := newOpenAISSEJSONDocumentScanner(scanner)
	restorer := apicompat.NewResponsesClientToolStreamRestorer(mapping)
	buffered := bufio.NewWriterSize(destination, 4*1024)
	pendingFields := make([]string, 0, 2)
	frameHadEventField := false
	frameEmitted := false

	writeLine := func(line string) error {
		if _, err := buffered.WriteString(line); err != nil {
			return err
	REDACTED
		return buffered.WriteByte('\n')
REDACTED
	writePendingFields := func(payload []byte, includeNonEvent bool) error {
		eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
		for _, field := range pendingFields {
			if _, isEvent := extractOpenAISSEEventLine(field); isEvent {
				if eventType != "" {
					if err := writeLine("event: " + eventType); err != nil {
						return err
				REDACTED
			REDACTED else if err := writeLine(field); err != nil {
					return err
			REDACTED
				continue
		REDACTED
			if includeNonEvent {
				if err := writeLine(field); err != nil {
					return err
			REDACTED
		REDACTED
	REDACTED
		return nil
REDACTED
	writePayloads := func(payloads [][]byte) error {
		for index, payload := range payloads {
			if index == 0 {
				if err := writePendingFields(payload, true); err != nil {
					return err
			REDACTED
		REDACTED else if frameHadEventField {
				eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
				if eventType != "" {
					if err := writeLine("event: " + eventType); err != nil {
						return err
				REDACTED
			REDACTED
		REDACTED
			if err := writeLine("data: " + string(payload)); err != nil {
				return err
		REDACTED
			if err := writeLine(""); err != nil {
				return err
		REDACTED
	REDACTED
		return buffered.Flush()
REDACTED

	for documents.Scan() {
		line := documents.Text()
		data, isData := extractOpenAISSEDataLine(line)
		if isData {
			payload := []byte(data)
			payloads := [][]byte{payloadREDACTED
			if json.Valid(payload) {
				var err error
				payloads, _, err = restorer.RestoreEvent(payload)
				if err != nil {
					_ = buffered.Flush()
					_ = destination.CloseWithError(fmt.Errorf("restore Grok Responses client tool event: %w", err))
					return
			REDACTED
		REDACTED
			if err := writePayloads(payloads); err != nil {
				_ = destination.CloseWithError(err)
				return
		REDACTED
			pendingFields = pendingFields[:0]
			frameHadEventField = false
			frameEmitted = true
			continue
	REDACTED

		if line == "" {
			if !frameEmitted {
				for _, field := range pendingFields {
					if err := writeLine(field); err != nil {
						_ = destination.CloseWithError(err)
						return
				REDACTED
			REDACTED
				if len(pendingFields) > 0 {
					if err := writeLine(""); err != nil {
						_ = destination.CloseWithError(err)
						return
				REDACTED
					if err := buffered.Flush(); err != nil {
						_ = destination.CloseWithError(err)
						return
				REDACTED
			REDACTED
		REDACTED
			pendingFields = pendingFields[:0]
			frameHadEventField = false
			frameEmitted = false
			continue
	REDACTED

		if _, isEvent := extractOpenAISSEEventLine(line); isEvent {
			frameHadEventField = true
	REDACTED
		pendingFields = append(pendingFields, line)
REDACTED

	for _, field := range pendingFields {
		if err := writeLine(field); err != nil {
			_ = destination.CloseWithError(err)
			return
	REDACTED
REDACTED
	if err := buffered.Flush(); err != nil {
		_ = destination.CloseWithError(err)
		return
REDACTED
	if err := documents.Err(); err != nil {
		_ = destination.CloseWithError(err)
		return
REDACTED
	_ = destination.Close()
REDACTED
