package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUsageRequestType(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		input   string
		want    RequestType
		wantErr bool
REDACTED

	cases := []testCase{
		{name: "unknown", input: "unknown", want: RequestTypeUnknownREDACTED,
		{name: "sync", input: "sync", want: RequestTypeSyncREDACTED,
		{name: "stream", input: "stream", want: RequestTypeStreamREDACTED,
		{name: "ws_v2", input: "ws_v2", want: RequestTypeWSV2REDACTED,
		{name: "case_insensitive", input: "WS_V2", want: RequestTypeWSV2REDACTED,
		{name: "trim_spaces", input: "  stream  ", want: RequestTypeStreamREDACTED,
		{name: "invalid", input: "xxx", wantErr: trueREDACTED,
REDACTED

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseUsageRequestType(tc.input)
			if tc.wantErr {
			REDACTED
				return
		REDACTED
		REDACTED
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED

func TestRequestTypeNormalizeAndString(t *testing.T) {
	t.Parallel()

	require.Equal(t, RequestTypeUnknown, RequestType(99).Normalize())
	require.Equal(t, "unknown", RequestType(99).String())
	require.Equal(t, "sync", RequestTypeSync.String())
	require.Equal(t, "stream", RequestTypeStream.String())
	require.Equal(t, "ws_v2", RequestTypeWSV2.String())
REDACTED

func TestRequestTypeFromLegacy(t *testing.T) {
	t.Parallel()

	require.Equal(t, RequestTypeWSV2, RequestTypeFromLegacy(false, true))
	require.Equal(t, RequestTypeStream, RequestTypeFromLegacy(true, false))
	require.Equal(t, RequestTypeSync, RequestTypeFromLegacy(false, false))
REDACTED

func TestApplyLegacyRequestFields(t *testing.T) {
	t.Parallel()

	stream, ws := ApplyLegacyRequestFields(RequestTypeSync, true, true)
	require.False(t, stream)
	require.False(t, ws)

	stream, ws = ApplyLegacyRequestFields(RequestTypeStream, false, true)
	require.True(t, stream)
	require.False(t, ws)

	stream, ws = ApplyLegacyRequestFields(RequestTypeWSV2, false, false)
	require.True(t, stream)
	require.True(t, ws)

	stream, ws = ApplyLegacyRequestFields(RequestTypeUnknown, true, false)
	require.True(t, stream)
	require.False(t, ws)
REDACTED

func TestUsageLogSyncRequestTypeAndLegacyFields(t *testing.T) {
	t.Parallel()

	log := &UsageLog{RequestType: RequestTypeWSV2, Stream: false, OpenAIWSMode: falseREDACTED
	log.SyncRequestTypeAndLegacyFields()

	require.Equal(t, RequestTypeWSV2, log.RequestType)
	require.True(t, log.Stream)
	require.True(t, log.OpenAIWSMode)
REDACTED

func TestUsageLogEffectiveRequestTypeFallback(t *testing.T) {
	t.Parallel()

	log := &UsageLog{RequestType: RequestTypeUnknown, Stream: true, OpenAIWSMode: trueREDACTED
	require.Equal(t, RequestTypeWSV2, log.EffectiveRequestType())
REDACTED

func TestUsageLogEffectiveRequestTypeNilReceiver(t *testing.T) {
	t.Parallel()

	var log *UsageLog
	require.Equal(t, RequestTypeUnknown, log.EffectiveRequestType())
REDACTED

func TestUsageLogSyncRequestTypeAndLegacyFieldsNilReceiver(t *testing.T) {
	t.Parallel()

	var log *UsageLog
	log.SyncRequestTypeAndLegacyFields()
REDACTED
