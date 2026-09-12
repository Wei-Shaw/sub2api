package audioduration

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

type wavSpec struct {
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
	byteRate      uint32 // 0 means derive from the other fields
	formatTag     uint16
	fmtSize       uint32 // 0 means 16
	dataBytes     int
	declaredData  *uint32 // nil means the real data length
	leadingChunks [][]byte
}

func chunk(id string, body []byte) []byte {
	out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	out = append(out, body...)
	if len(body)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func buildWAV(spec wavSpec) []byte {
	if spec.formatTag == 0 {
		spec.formatTag = 1
	}
	if spec.fmtSize == 0 {
		spec.fmtSize = 16
	}
	byteRate := spec.byteRate
	if byteRate == 0 {
		byteRate = spec.sampleRate * uint32(spec.channels) * uint32(spec.bitsPerSample) / 8
	}
	fmtBody := make([]byte, spec.fmtSize)
	binary.LittleEndian.PutUint16(fmtBody[0:], spec.formatTag)
	binary.LittleEndian.PutUint16(fmtBody[2:], spec.channels)
	binary.LittleEndian.PutUint32(fmtBody[4:], spec.sampleRate)
	binary.LittleEndian.PutUint32(fmtBody[8:], byteRate)
	binary.LittleEndian.PutUint16(fmtBody[12:], spec.channels*spec.bitsPerSample/8)
	binary.LittleEndian.PutUint16(fmtBody[14:], spec.bitsPerSample)

	body := []byte("WAVE")
	for _, leading := range spec.leadingChunks {
		body = append(body, leading...)
	}
	body = append(body, chunk("fmt ", fmtBody)...)
	body = append(body, "data"...)
	declared := uint32(spec.dataBytes)
	if spec.declaredData != nil {
		declared = *spec.declaredData
	}
	body = binary.LittleEndian.AppendUint32(body, declared)
	body = append(body, make([]byte, spec.dataBytes)...)

	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	return append(out, body...)
}

func u32p(v uint32) *uint32 { return &v }

func TestParseWAV(t *testing.T) {
	tests := []struct {
		name    string
		spec    wavSpec
		seconds float64
	}{
		{name: "16k mono 16-bit one second", spec: wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 32000}, seconds: 1},
		{name: "48k stereo 24-bit half second", spec: wavSpec{channels: 2, sampleRate: 48000, bitsPerSample: 24, dataBytes: 144000}, seconds: 0.5},
		{name: "8k mono 8-bit", spec: wavSpec{channels: 1, sampleRate: 8000, bitsPerSample: 8, dataBytes: 4000}, seconds: 0.5},
		{name: "extensible format tag", spec: wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, formatTag: 0xFFFE, fmtSize: 40, dataBytes: 16000}, seconds: 0.5},
		{name: "odd sized LIST chunk before fmt", spec: wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 32000, leadingChunks: [][]byte{chunk("LIST", []byte("INFOxyz"))}}, seconds: 1},
		{name: "declared data size zero uses remaining bytes", spec: wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 64000, declaredData: u32p(0)}, seconds: 2},
		{name: "declared data size 0xFFFFFFFF uses remaining bytes", spec: wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 64000, declaredData: u32p(0xFFFFFFFF)}, seconds: 2},
		{name: "declared data size beyond payload uses remaining bytes", spec: wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 32000, declaredData: u32p(320000)}, seconds: 1},
		{name: "zero byte rate is derived from fields", spec: wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 32000, byteRate: 0}, seconds: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildWAV(tt.spec)
			seconds, ok := ParseWAV(data)
			require.True(t, ok)
			require.InDelta(t, tt.seconds, seconds, 1e-9)
		})
	}
}

func TestParseWAVSetsByteRateFromHeaderNotFields(t *testing.T) {
	// A writer that declares a byte rate must win over the derived value.
	data := buildWAV(wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, byteRate: 64000, dataBytes: 64000})
	seconds, ok := ParseWAV(data)
	require.True(t, ok)
	require.InDelta(t, 1, seconds, 1e-9)
}

func TestParseWAVRejectsUnusableInput(t *testing.T) {
	valid := buildWAV(wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 32000})
	tests := []struct {
		name string
		data []byte
	}{
		{name: "nil", data: nil},
		{name: "not riff", data: []byte("ID3\x03\x00\x00\x00\x00\x00\x00abcdefghijklmnop")},
		{name: "riff but not wave", data: append([]byte("RIFF\x10\x00\x00\x00AVI "), make([]byte, 16)...)},
		{name: "truncated header", data: valid[:20]},
		{name: "fmt chunk without data chunk", data: valid[:36]},
		{name: "data chunk header without payload", data: valid[:44]},
		{name: "fmt chunk too short", data: func() []byte {
			b := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+8+8+8))...)
			b = append(b, "WAVE"...)
			b = append(b, chunk("fmt ", make([]byte, 8))...)
			return append(b, chunk("data", make([]byte, 8))...)
		}()},
		{name: "unrecoverable zero rates", data: buildWAV(wavSpec{channels: 0, sampleRate: 0, bitsPerSample: 0, dataBytes: 100})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := ParseWAV(tt.data)
			require.False(t, ok)
		})
	}
}

func TestMeasure(t *testing.T) {
	t.Run("wav is exact", func(t *testing.T) {
		seconds, exact := Measure(buildWAV(wavSpec{channels: 1, sampleRate: 16000, bitsPerSample: 16, dataBytes: 48000}))
		require.True(t, exact)
		require.InDelta(t, 1.5, seconds, 1e-9)
	})
	t.Run("other containers are estimated from size", func(t *testing.T) {
		data := make([]byte, 64000)
		copy(data, "ID3")
		seconds, exact := Measure(data)
		require.False(t, exact)
		require.InDelta(t, float64(64000*8)/float64(AssumedBitsPerSecond), seconds, 1e-9)
	})
	t.Run("empty input", func(t *testing.T) {
		seconds, exact := Measure(nil)
		require.False(t, exact)
		require.Zero(t, seconds)
	})
}
