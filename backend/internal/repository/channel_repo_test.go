//go:build unit

package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// --- marshalModelMapping ---

func TestMarshalModelMapping(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]map[string]string
		wantJSON string // expected JSON output (exact match)
REDACTED{
		{
			name:     "empty map",
			input:    map[string]map[string]string{REDACTED,
			wantJSON: "{REDACTED",
	REDACTED,
		{
			name:     "nil map",
			input:    nil,
			wantJSON: "{REDACTED",
	REDACTED,
		{
			name: "populated map",
			input: map[string]map[string]string{
				"openai": {"gpt-4": "gpt-4-turbo"REDACTED,
		REDACTED,
	REDACTED,
		{
			name: "nested values",
			input: map[string]map[string]string{
				"openai":    {"*": "gpt-5.4"REDACTED,
				"anthropic": {"claude-old": "claude-new"REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalModelMapping(tt.input)
		REDACTED

			if tt.wantJSON != "" {
				require.Equal(t, []byte(tt.wantJSON), result)
		REDACTED else {
				// round-trip: unmarshal and compare with input
				var parsed map[string]map[string]string
				require.NoError(t, json.Unmarshal(result, &parsed))
				require.Equal(t, tt.input, parsed)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// --- unmarshalModelMapping ---

func TestUnmarshalModelMapping(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantNil bool
		want    map[string]map[string]string
REDACTED{
		{
			name:    "nil data",
			input:   nil,
			wantNil: true,
	REDACTED,
		{
			name:    "empty data",
			input:   []byte{REDACTED,
			wantNil: true,
	REDACTED,
		{
			name:    "invalid JSON",
			input:   []byte("not-json"),
			wantNil: true,
	REDACTED,
		{
			name:    "type error - number",
			input:   []byte("42"),
			wantNil: true,
	REDACTED,
		{
			name:    "type error - array",
			input:   []byte("[1,2,3]"),
			wantNil: true,
	REDACTED,
		{
			name:  "valid JSON",
			input: []byte(`{"openai":{"gpt-4":"gpt-4-turbo"REDACTED,"anthropic":{"old":"new"REDACTEDREDACTED`),
			want: map[string]map[string]string{
				"openai":    {"gpt-4": "gpt-4-turbo"REDACTED,
				"anthropic": {"old": "new"REDACTED,
		REDACTED,
	REDACTED,
		{
			name:  "empty object",
			input: []byte("{REDACTED"),
			want:  map[string]map[string]string{REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unmarshalModelMapping(tt.input)
			if tt.wantNil {
				require.Nil(t, result)
		REDACTED else {
				require.NotNil(t, result)
				require.Equal(t, tt.want, result)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// --- escapeLike ---

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
REDACTED{
		{
			name:  "no special chars",
			input: "hello",
			want:  "hello",
	REDACTED,
		{
			name:  "backslash",
			input: `a\b`,
			want:  `a\\b`,
	REDACTED,
		{
			name:  "percent",
			input: "50%",
			want:  `50\%`,
	REDACTED,
		{
			name:  "underscore",
			input: "a_b",
			want:  `a\_b`,
	REDACTED,
		{
			name:  "all special chars",
			input: `a\b%c_d`,
			want:  `a\\b\%c\_d`,
	REDACTED,
		{
			name:  "empty string",
			input: "",
			want:  "",
	REDACTED,
		{
			name:  "consecutive special chars",
			input: "%_%",
			want:  `\%\_\%`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, escapeLike(tt.input))
	REDACTED)
REDACTED
REDACTED

// --- isUniqueViolation ---

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
REDACTED{
		{
			name: "unique violation code 23505",
			err:  &pq.Error{Code: "23505"REDACTED,
			want: true,
	REDACTED,
		{
			name: "different pq error code",
			err:  &pq.Error{Code: "23503"REDACTED,
			want: false,
	REDACTED,
		{
			name: "non-pq error",
			err:  errors.New("some generic error"),
			want: false,
	REDACTED,
		{
			name: "typed nil pq.Error",
			err: func() error {
				var pqErr *pq.Error
				return pqErr
		REDACTED(),
			want: false,
	REDACTED,
		{
			name: "bare nil",
			err:  nil,
			want: false,
	REDACTED,
		{
			name: "wrapped pq error with 23505",
			err:  fmt.Errorf("wrapped: %w", &pq.Error{Code: "23505"REDACTED),
			want: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isUniqueViolation(tt.err))
	REDACTED)
REDACTED
REDACTED
