package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIStreamEventIsTerminalWithTypeMatchesExistingSemantics(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
REDACTED{
		{name: "empty", data: "", want: falseREDACTED,
		{name: "whitespace", data: " \t ", want: falseREDACTED,
		{name: "done", data: " [DONE] ", want: trueREDACTED,
		{name: "JSON outer whitespace", data: " \n\t {\"type\":\"response.completed\"REDACTED \r\n", want: trueREDACTED,
		{name: "completed", data: `{"type":"response.completed"REDACTED`, want: trueREDACTED,
		{name: "response done", data: `{"type":"response.done"REDACTED`, want: trueREDACTED,
		{name: "failed", data: `{"type":"response.failed"REDACTED`, want: trueREDACTED,
		{name: "incomplete", data: `{"type":"response.incomplete"REDACTED`, want: trueREDACTED,
		{name: "cancelled", data: `{"type":"response.cancelled"REDACTED`, want: trueREDACTED,
		{name: "canceled", data: `{"type":"response.canceled"REDACTED`, want: trueREDACTED,
		{name: "delta", data: `{"type":"response.output_text.delta"REDACTED`, want: falseREDACTED,
		{name: "invalid JSON", data: `{"type":`, want: falseREDACTED,
		{name: "terminal with trailing garbage", data: `{"type":"response.completed"REDACTED trailing`, want: trueREDACTED,
		{name: "nonterminal with trailing garbage", data: `{"type":"response.output_text.delta"REDACTED trailing`, want: falseREDACTED,
		{name: "type whitespace remains nonterminal", data: `{"type":" response.completed "REDACTED`, want: falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventType := gjson.GetBytes([]byte(tt.data), "type").String()
			got := openAIStreamEventIsTerminalWithType(tt.data, eventType)

			require.Equal(t, tt.want, got)
			require.Equal(t, openAIStreamEventIsTerminal(tt.data), got)
	REDACTED)
REDACTED
REDACTED

var (
	benchmarkOpenAIResponseSSEEventTypeSink string
	benchmarkOpenAIResponseSSETerminalSink  bool
)

func BenchmarkOpenAIResponseSSETypeExtraction(b *testing.B) {
	data := `{"type":"response.output_text.delta","sequence_number":42,"delta":"streaming response benchmark payload"REDACTED`
	dataBytes := []byte(data)

	b.Run("legacy double parse", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(dataBytes)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkOpenAIResponseSSETerminalSink = openAIStreamEventIsTerminal(data)
			benchmarkOpenAIResponseSSEEventTypeSink = strings.TrimSpace(gjson.GetBytes(dataBytes, "type").String())
	REDACTED
REDACTED)

	b.Run("reused single parse", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(dataBytes)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eventTypeRaw := gjson.GetBytes(dataBytes, "type").String()
			benchmarkOpenAIResponseSSEEventTypeSink = strings.TrimSpace(eventTypeRaw)
			benchmarkOpenAIResponseSSETerminalSink = openAIStreamEventIsTerminalWithType(data, eventTypeRaw)
	REDACTED
REDACTED)
REDACTED
