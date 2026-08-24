//go:build unit

package service

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apiz"
	"github.com/stretchr/testify/require"
)

func TestVideoPrechargeDurations(t *testing.T) {
	t.Run("generic auto holds thirty seconds", func(t *testing.T) {
		billable, stored := videoPrechargeDurations(
			&Account{Platform: PlatformFal},
			map[string]any{"duration": "auto"},
			0,
		)
		require.Equal(t, 30, billable)
		require.Zero(t, stored)
	})

	t.Run("apiz auto uses prepared duration", func(t *testing.T) {
		account := &Account{Platform: PlatformApiz}
		payload := prepareVideoRequestPayload(account, "model", map[string]any{"duration": "auto"})
		require.Equal(t, apiz.AutoDurationFallbackSeconds, payload["duration"])

		billable, stored := videoPrechargeDurations(account, payload, 0)
		require.Equal(t, apiz.AutoDurationFallbackSeconds, billable)
		require.Equal(t, apiz.AutoDurationFallbackSeconds, stored)
	})
}

func TestParseVideoDurationSecondsSupportsWebM(t *testing.T) {
	scale := []byte{0x0f, 0x42, 0x40} // 1,000,000ns
	duration := make([]byte, 8)
	binary.BigEndian.PutUint64(duration, math.Float64bits(8_250))
	info := append(webmTestElement([]byte{0x2a, 0xd7, 0xb1}, scale), webmTestElement([]byte{0x44, 0x89}, duration)...)
	data := webmTestElement([]byte{0x18, 0x53, 0x80, 0x67}, webmTestElement([]byte{0x15, 0x49, 0xa9, 0x66}, info))

	seconds, err := parseVideoDurationSeconds(data)

	require.NoError(t, err)
	require.Equal(t, 9, seconds)
}

func TestParseMP4DurationSeconds(t *testing.T) {
	t.Run("version zero rounds partial seconds up", func(t *testing.T) {
		mvhd := make([]byte, 20)
		binary.BigEndian.PutUint32(mvhd[12:16], 1_000)
		binary.BigEndian.PutUint32(mvhd[16:20], 8_001)
		requireMP4Duration(t, mvhd, 9)
	})

	t.Run("version one supports 64 bit duration", func(t *testing.T) {
		mvhd := make([]byte, 32)
		mvhd[0] = 1
		binary.BigEndian.PutUint32(mvhd[20:24], 90_000)
		binary.BigEndian.PutUint64(mvhd[24:32], 900_000)
		requireMP4Duration(t, mvhd, 10)
	})

	t.Run("missing movie header fails", func(t *testing.T) {
		_, err := parseMP4DurationSeconds(mp4TestBox("free", []byte("data")))
		require.ErrorIs(t, err, errVideoDurationNotFound)
	})

	t.Run("falls back to media header when movie duration is empty", func(t *testing.T) {
		mvhd := make([]byte, 20)
		mdhd := make([]byte, 20)
		binary.BigEndian.PutUint32(mdhd[12:16], 1_000)
		binary.BigEndian.PutUint32(mdhd[16:20], 12_000)
		mdia := mp4TestBox("mdia", mp4TestBox("mdhd", mdhd))
		trak := mp4TestBox("trak", mdia)
		moovPayload := append(mp4TestBox("mvhd", mvhd), trak...)

		duration, err := parseMP4DurationSeconds(mp4TestBox("moov", moovPayload))

		require.NoError(t, err)
		require.Equal(t, 12, duration)
	})
}

func requireMP4Duration(t *testing.T, mvhd []byte, expected int) {
	t.Helper()
	data := mp4TestBox("moov", mp4TestBox("mvhd", mvhd))
	duration, err := parseMP4DurationSeconds(data)
	require.NoError(t, err)
	require.Equal(t, expected, duration)
}

func mp4TestBox(boxType string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[:4], uint32(len(box)))
	copy(box[4:8], boxType)
	copy(box[8:], payload)
	return box
}

func webmTestElement(id, payload []byte) []byte {
	if len(payload) >= 127 {
		panic("test payload too large")
	}
	element := append([]byte(nil), id...)
	element = append(element, byte(0x80|len(payload)))
	return append(element, payload...)
}
