//go:build unit

package admin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotCache_SetAndGet(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)

	entry := c.Set("key1", map[string]string{"hello": "world"REDACTED)
	require.NotEmpty(t, entry.ETag)
	require.NotNil(t, entry.Payload)

	got, ok := c.Get("key1")
	require.True(t, ok)
	require.Equal(t, entry.ETag, got.ETag)
REDACTED

func TestSnapshotCache_Expiration(t *testing.T) {
	c := newSnapshotCache(1 * time.Millisecond)

	c.Set("key1", "value")
	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("key1")
	require.False(t, ok, "expired entry should not be returned")
REDACTED

func TestSnapshotCache_GetEmptyKey(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	_, ok := c.Get("")
	require.False(t, ok)
REDACTED

func TestSnapshotCache_GetMiss(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	_, ok := c.Get("nonexistent")
	require.False(t, ok)
REDACTED

func TestSnapshotCache_NilReceiver(t *testing.T) {
	var c *snapshotCache
	_, ok := c.Get("key")
	require.False(t, ok)

	entry := c.Set("key", "value")
	require.Empty(t, entry.ETag)
REDACTED

func TestSnapshotCache_SetEmptyKey(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)

	// Set with empty key should return entry but not store it
	entry := c.Set("", "value")
	require.NotEmpty(t, entry.ETag)

	_, ok := c.Get("")
	require.False(t, ok)
REDACTED

func TestSnapshotCache_DefaultTTL(t *testing.T) {
	c := newSnapshotCache(0)
	require.Equal(t, 30*time.Second, c.ttl)

	c2 := newSnapshotCache(-1 * time.Second)
	require.Equal(t, 30*time.Second, c2.ttl)
REDACTED

func TestSnapshotCache_ETagDeterministic(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	payload := map[string]int{"a": 1, "b": 2REDACTED

	entry1 := c.Set("k1", payload)
	entry2 := c.Set("k2", payload)
	require.Equal(t, entry1.ETag, entry2.ETag, "same payload should produce same ETag")
REDACTED

func TestSnapshotCache_ETagFormat(t *testing.T) {
	c := newSnapshotCache(5 * time.Second)
	entry := c.Set("k", "test")
	// ETag should be quoted hex string: "abcdef..."
	require.True(t, len(entry.ETag) > 2)
	require.Equal(t, byte('"'), entry.ETag[0])
	require.Equal(t, byte('"'), entry.ETag[len(entry.ETag)-1])
REDACTED

func TestBuildETagFromAny_UnmarshalablePayload(t *testing.T) {
	// channels are not JSON-serializable
	etag := buildETagFromAny(make(chan int))
	require.Empty(t, etag)
REDACTED

func TestParseBoolQueryWithDefault(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		def  bool
		want bool
REDACTED{
		{"empty returns default true", "", true, trueREDACTED,
		{"empty returns default false", "", false, falseREDACTED,
		{"1", "1", false, trueREDACTED,
		{"true", "true", false, trueREDACTED,
		{"TRUE", "TRUE", false, trueREDACTED,
		{"yes", "yes", false, trueREDACTED,
		{"on", "on", false, trueREDACTED,
		{"0", "0", true, falseREDACTED,
		{"false", "false", true, falseREDACTED,
		{"FALSE", "FALSE", true, falseREDACTED,
		{"no", "no", true, falseREDACTED,
		{"off", "off", true, falseREDACTED,
		{"whitespace trimmed", "  true  ", false, trueREDACTED,
		{"unknown returns default true", "maybe", true, trueREDACTED,
		{"unknown returns default false", "maybe", false, falseREDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBoolQueryWithDefault(tc.raw, tc.def)
			require.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED
