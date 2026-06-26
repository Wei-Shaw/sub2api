package service

import (
	"bytes"
	"encoding/binary"
	"math"
)

const (
	ebmlIDHeader        = 0x1A45DFA3
	ebmlIDSegment       = 0x18538067
	ebmlIDInfo          = 0x1549A966
	ebmlIDTimecodeScale = 0x2AD7B1
	ebmlIDDuration      = 0x4489
	ebmlIDTracks        = 0x1654AE6B
	ebmlIDTrackEntry    = 0xAE
	ebmlIDTrackNumber   = 0xD7
	ebmlIDTrackType     = 0x83
	ebmlIDCodecID       = 0x86
	ebmlIDAudio         = 0xE1
	ebmlIDCluster       = 0x1F43B675
	ebmlIDClusterTime   = 0xE7
	ebmlIDSimpleBlock   = 0xA3
	ebmlIDBlockGroup    = 0xA0
	ebmlIDBlock         = 0xA1

	webmDefaultTimecodeScale = uint64(1000000)
)

func ExtractBillableAudioDurationSeconds(data []byte) (int, bool) {
	if len(data) == 0 {
		return 0, false
	}
	return parseAudioDurationSeconds(data)
}

func ceilPositiveSeconds(value float64) int {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return int(math.Ceil(value))
}

func parseAudioDurationSeconds(data []byte) (int, bool) {
	switch {
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WAVE")):
		return parseWAVDurationSeconds(data)
	case len(data) >= 10 && bytes.HasPrefix(data, []byte("ID3")),
		len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0:
		return parseMP3DurationSeconds(data)
	case len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")):
		return parseMP4DurationSeconds(data)
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return parseWebMDurationSeconds(data)
	default:
		return 0, false
	}
}

func parseWAVDurationSeconds(data []byte) (int, bool) {
	var byteRate uint32
	var dataSize uint32
	for offset := 12; offset+8 <= len(data); {
		chunkID := string(data[offset : offset+4])
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		payload := offset + 8
		next := payload + int(chunkSize)
		if next > len(data) {
			return 0, false
		}
		switch chunkID {
		case "fmt ":
			if chunkSize >= 16 {
				byteRate = binary.LittleEndian.Uint32(data[payload+8 : payload+12])
			}
		case "data":
			dataSize = chunkSize
		}
		if byteRate > 0 && dataSize > 0 {
			return ceilPositiveSeconds(float64(dataSize) / float64(byteRate)), true
		}
		offset = next + int(chunkSize%2)
	}
	return 0, false
}

func parseMP3DurationSeconds(data []byte) (int, bool) {
	offset := skipID3v2(data)
	totalSeconds := 0.0
	frames := 0
	for offset+4 <= len(data) {
		frameLen, samples, sampleRate, ok := parseMP3FrameHeader(data[offset : offset+4])
		if !ok || frameLen <= 0 || offset+frameLen > len(data) {
			offset++
			continue
		}
		totalSeconds += float64(samples) / float64(sampleRate)
		frames++
		offset += frameLen
	}
	if frames == 0 {
		return 0, false
	}
	return ceilPositiveSeconds(totalSeconds), true
}

func skipID3v2(data []byte) int {
	if len(data) < 10 || !bytes.HasPrefix(data, []byte("ID3")) {
		return 0
	}
	size := int(data[6]&0x7f)<<21 | int(data[7]&0x7f)<<14 | int(data[8]&0x7f)<<7 | int(data[9]&0x7f)
	if 10+size >= len(data) {
		return 0
	}
	return 10 + size
}

func parseMP3FrameHeader(h []byte) (frameLen int, samples int, sampleRate int, ok bool) {
	if len(h) < 4 || h[0] != 0xff || h[1]&0xe0 != 0xe0 {
		return 0, 0, 0, false
	}
	versionID := (h[1] >> 3) & 0x03
	layerID := (h[1] >> 1) & 0x03
	bitrateIdx := (h[2] >> 4) & 0x0f
	sampleIdx := (h[2] >> 2) & 0x03
	padding := int((h[2] >> 1) & 0x01)
	if versionID == 1 || layerID == 0 || bitrateIdx == 0 || bitrateIdx == 15 || sampleIdx == 3 {
		return 0, 0, 0, false
	}
	sampleRate = mp3SampleRate(versionID, sampleIdx)
	bitrate := mp3BitrateKbps(versionID, layerID, bitrateIdx)
	if sampleRate <= 0 || bitrate <= 0 {
		return 0, 0, 0, false
	}
	samples = mp3SamplesPerFrame(versionID, layerID)
	if layerID == 3 {
		frameLen = (12*bitrate*1000/sampleRate + padding) * 4
	} else if layerID == 1 && versionID != 3 {
		frameLen = 72*bitrate*1000/sampleRate + padding
	} else {
		frameLen = 144*bitrate*1000/sampleRate + padding
	}
	return frameLen, samples, sampleRate, true
}

func mp3SampleRate(versionID byte, sampleIdx byte) int {
	base := []int{44100, 48000, 32000}[sampleIdx]
	switch versionID {
	case 3:
		return base
	case 2:
		return base / 2
	case 0:
		return base / 4
	default:
		return 0
	}
}

func mp3BitrateKbps(versionID, layerID, idx byte) int {
	tables := map[byte]map[byte][]int{
		3: {
			3: {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
			2: {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
			1: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
		},
		2: {
			3: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
			2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
			1: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		},
		0: {
			3: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
			2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
			1: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		},
	}
	return tables[versionID][layerID][idx]
}

func mp3SamplesPerFrame(versionID, layerID byte) int {
	switch layerID {
	case 3:
		return 384
	case 2:
		return 1152
	case 1:
		if versionID == 3 {
			return 1152
		}
		return 576
	default:
		return 0
	}
}

func parseMP4DurationSeconds(data []byte) (int, bool) {
	duration, ok := scanMP4Duration(data, 0)
	if !ok {
		return 0, false
	}
	return ceilPositiveSeconds(duration), true
}

func scanMP4Duration(data []byte, depth int) (float64, bool) {
	if depth > 8 {
		return 0, false
	}
	for offset := 0; offset+8 <= len(data); {
		size := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		boxType := string(data[offset+4 : offset+8])
		header := 8
		switch size {
		case 1:
			if offset+16 > len(data) {
				return 0, false
			}
			size = binary.BigEndian.Uint64(data[offset+8 : offset+16])
			header = 16
		case 0:
			size = uint64(len(data) - offset)
		}
		if size < uint64(header) || offset+int(size) > len(data) {
			return 0, false
		}
		payload := data[offset+header : offset+int(size)]
		switch boxType {
		case "mvhd", "mdhd":
			if duration, ok := parseMP4MovieHeaderDuration(payload); ok {
				return duration, true
			}
		case "moov", "trak", "mdia":
			if duration, ok := scanMP4Duration(payload, depth+1); ok {
				return duration, true
			}
		}
		offset += int(size)
	}
	return 0, false
}

func parseMP4MovieHeaderDuration(payload []byte) (float64, bool) {
	if len(payload) < 20 {
		return 0, false
	}
	version := payload[0]
	if version == 1 {
		if len(payload) < 32 {
			return 0, false
		}
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		if timescale == 0 || duration == 0 {
			return 0, false
		}
		return float64(duration) / float64(timescale), true
	}
	timescale := binary.BigEndian.Uint32(payload[12:16])
	duration := binary.BigEndian.Uint32(payload[16:20])
	if timescale == 0 || duration == 0 {
		return 0, false
	}
	return float64(duration) / float64(timescale), true
}

func parseWebMDurationSeconds(data []byte) (int, bool) {
	if len(data) < 4 || binary.BigEndian.Uint32(data[:4]) != ebmlIDHeader {
		return 0, false
	}
	duration, ok := scanWebMDuration(data)
	if !ok {
		return 0, false
	}
	return ceilPositiveSeconds(duration), true
}

func scanWebMDuration(data []byte) (float64, bool) {
	offset := 0
	scale := webmDefaultTimecodeScale
	for offset < len(data) {
		id, idLen, ok := readEBMLVintID(data[offset:])
		if !ok {
			break
		}
		offset += idLen
		size, sizeLen, unknown, ok := readEBMLVintSize(data[offset:])
		if !ok {
			break
		}
		offset += sizeLen
		if unknown {
			size = uint64(len(data) - offset)
		}
		if size > uint64(len(data)-offset) {
			break
		}
		payload := data[offset : offset+int(size)]
		switch id {
		case ebmlIDInfo:
			if duration, found := scanWebMInfo(payload, &scale); found {
				return duration, true
			}
		case ebmlIDSegment:
			if duration, found := scanWebMDuration(payload); found {
				return duration, true
			}
		}
		offset += int(size)
	}
	return scanWebMClusterDuration(data, scale)
}

func scanWebMInfo(data []byte, scale *uint64) (float64, bool) {
	offset := 0
	for offset < len(data) {
		id, idLen, ok := readEBMLVintID(data[offset:])
		if !ok {
			break
		}
		offset += idLen
		size, sizeLen, _, ok := readEBMLVintSize(data[offset:])
		if !ok || size > uint64(len(data)-offset-sizeLen) {
			break
		}
		offset += sizeLen
		payload := data[offset : offset+int(size)]
		switch id {
		case ebmlIDTimecodeScale:
			if value, ok := readUnsignedEBML(payload); ok && value > 0 {
				*scale = value
			}
		case ebmlIDDuration:
			if duration, ok := readFloatEBML(payload); ok && duration > 0 {
				return duration * float64(*scale) / 1e9, true
			}
		}
		offset += int(size)
	}
	return 0, false
}

func scanWebMClusterDuration(data []byte, scale uint64) (float64, bool) {
	offset := 0
	lastTimecode := uint64(0)
	found := false
	for offset < len(data) {
		id, idLen, ok := readEBMLVintID(data[offset:])
		if !ok {
			break
		}
		offset += idLen
		size, sizeLen, unknown, ok := readEBMLVintSize(data[offset:])
		if !ok {
			break
		}
		offset += sizeLen
		if unknown {
			size = uint64(len(data) - offset)
		}
		if size > uint64(len(data)-offset) {
			break
		}
		if id == ebmlIDCluster {
			if tc, ok := scanWebMClusterTimecode(data[offset : offset+int(size)]); ok {
				lastTimecode = tc
				found = true
			}
		}
		offset += int(size)
	}
	if !found {
		return 0, false
	}
	return float64(lastTimecode*scale) / 1e9, true
}

func scanWebMClusterTimecode(data []byte) (uint64, bool) {
	offset := 0
	for offset < len(data) {
		id, idLen, ok := readEBMLVintID(data[offset:])
		if !ok {
			break
		}
		offset += idLen
		size, sizeLen, _, ok := readEBMLVintSize(data[offset:])
		if !ok || size > uint64(len(data)-offset-sizeLen) {
			break
		}
		offset += sizeLen
		payload := data[offset : offset+int(size)]
		if id == ebmlIDClusterTime {
			return readUnsignedEBML(payload)
		}
		offset += int(size)
	}
	return 0, false
}

func readEBMLVintID(data []byte) (uint64, int, bool) {
	if len(data) == 0 {
		return 0, 0, false
	}
	first := data[0]
	mask := byte(0x80)
	length := 1
	for length <= 4 && first&mask == 0 {
		mask >>= 1
		length++
	}
	if length > 4 || length > len(data) {
		return 0, 0, false
	}
	value := uint64(0)
	for i := 0; i < length; i++ {
		value = (value << 8) | uint64(data[i])
	}
	return value, length, true
}

func readEBMLVintSize(data []byte) (uint64, int, bool, bool) {
	if len(data) == 0 {
		return 0, 0, false, false
	}
	first := data[0]
	mask := byte(0x80)
	length := 1
	for length <= 8 && first&mask == 0 {
		mask >>= 1
		length++
	}
	if length > 8 || length > len(data) {
		return 0, 0, false, false
	}
	value := uint64(first & ^mask)
	for i := 1; i < length; i++ {
		value = (value << 8) | uint64(data[i])
	}
	unknown := value == (uint64(1)<<(uint(length)*7))-1
	return value, length, unknown, true
}

func readUnsignedEBML(data []byte) (uint64, bool) {
	if len(data) == 0 || len(data) > 8 {
		return 0, false
	}
	value := uint64(0)
	for _, b := range data {
		value = (value << 8) | uint64(b)
	}
	return value, true
}

func readFloatEBML(data []byte) (float64, bool) {
	switch len(data) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(data))), true
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(data)), true
	default:
		return 0, false
	}
}
