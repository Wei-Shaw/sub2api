// Package audioduration measures the playback length of uploaded audio without
// decoding it: RIFF/WAVE headers are parsed exactly and every other container
// is estimated from its byte size.
package audioduration

import "encoding/binary"

// AssumedBitsPerSecond is the bitrate used to estimate the duration of
// non-WAV uploads from their size. 128 kbps is the common default of browser
// MediaRecorder and dictation encoders; lower-bitrate files are under-estimated.
const AssumedBitsPerSecond = 128_000

const (
	maxChunkScan      = 64
	maxHeaderScanSize = 1 << 20
	minFmtChunkSize   = 16
)

// Measure returns the audio length in seconds and whether it was read exactly
// from the container header rather than estimated.
func Measure(data []byte) (seconds float64, exact bool) {
	if seconds, ok := ParseWAV(data); ok {
		return seconds, true
	}
	if len(data) == 0 {
		return 0, false
	}
	return float64(len(data)) * 8 / float64(AssumedBitsPerSecond), false
}

// ParseWAV returns the length of a RIFF/WAVE payload. ok is false when the
// payload is not WAVE or its header cannot yield a duration.
func ParseWAV(data []byte) (seconds float64, ok bool) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, false
	}

	var (
		channels      uint32
		sampleRate    uint32
		byteRate      uint32
		bitsPerSample uint32
		haveFmt       bool
		dataBytes     int64 = -1
	)
	total := int64(len(data))
	offset := int64(12)
	for i := 0; i < maxChunkScan && offset+8 <= total && offset <= maxHeaderScanSize; i++ {
		id := string(data[offset : offset+4])
		size := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		bodyStart := offset + 8
		switch id {
		case "fmt ":
			if size < minFmtChunkSize || bodyStart+minFmtChunkSize > total {
				return 0, false
			}
			body := data[bodyStart : bodyStart+minFmtChunkSize]
			// Only the first 16 bytes are read, so WAVE_FORMAT_EXTENSIBLE
			// (tag 0xFFFE, 40-byte chunk) needs no special handling.
			channels = uint32(binary.LittleEndian.Uint16(body[2:4]))
			sampleRate = binary.LittleEndian.Uint32(body[4:8])
			byteRate = binary.LittleEndian.Uint32(body[8:12])
			bitsPerSample = uint32(binary.LittleEndian.Uint16(body[14:16]))
			haveFmt = true
		case "data":
			available := total - bodyStart
			// Streaming writers leave the size as 0 or 0xFFFFFFFF and truncated
			// uploads declare more than they carry; the bytes present are the truth.
			if size == 0 || size == 0xFFFFFFFF || size > available {
				size = available
			}
			dataBytes = size
		}
		if dataBytes >= 0 {
			break
		}
		offset = bodyStart + size + (size & 1)
	}
	if !haveFmt || dataBytes <= 0 {
		return 0, false
	}
	if byteRate == 0 {
		byteRate = sampleRate * channels * bitsPerSample / 8
	}
	if byteRate == 0 {
		return 0, false
	}
	return float64(dataBytes) / float64(byteRate), true
}
