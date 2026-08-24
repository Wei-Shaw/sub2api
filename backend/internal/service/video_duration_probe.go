package service

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

var errVideoDurationNotFound = errors.New("video duration metadata not found")

func parseVideoDurationSeconds(data []byte) (int, error) {
	if duration, err := parseMP4DurationSeconds(data); err == nil {
		return duration, nil
	}
	if duration, err := parseWebMDurationSeconds(data); err == nil {
		return duration, nil
	}
	return 0, errVideoDurationNotFound
}

// parseMP4DurationSeconds reads the ISO BMFF movie header and rounds a partial
// second up because video pricing is expressed in whole output seconds.
func parseMP4DurationSeconds(data []byte) (int, error) {
	duration, err := findMP4MovieDuration(data)
	if err != nil {
		return 0, err
	}
	seconds := int(math.Ceil(duration))
	if seconds <= 0 {
		return 0, errVideoDurationNotFound
	}
	return seconds, nil
}

func findMP4MovieDuration(data []byte) (float64, error) {
	bestDuration := float64(0)
	for offset := 0; offset+8 <= len(data); {
		size, headerSize, boxType, err := mp4BoxHeader(data[offset:])
		if err != nil {
			return 0, err
		}
		end := offset + size
		if end > len(data) || size < headerSize {
			return 0, fmt.Errorf("invalid mp4 box %q size", boxType)
		}
		payload := data[offset+headerSize : end]
		switch boxType {
		case "mvhd", "mdhd":
			if duration, parseErr := parseMP4MovieHeader(payload); parseErr == nil && duration > bestDuration {
				bestDuration = duration
			}
		case "moov", "trak", "mdia":
			if duration, nestedErr := findMP4MovieDuration(payload); nestedErr == nil && duration > bestDuration {
				bestDuration = duration
			}
		}
		offset = end
	}
	if bestDuration > 0 {
		return bestDuration, nil
	}
	return 0, errVideoDurationNotFound
}

func mp4BoxHeader(data []byte) (size int, headerSize int, boxType string, err error) {
	if len(data) < 8 {
		return 0, 0, "", errVideoDurationNotFound
	}
	rawSize := binary.BigEndian.Uint32(data[:4])
	boxType = string(data[4:8])
	headerSize = 8
	switch rawSize {
	case 0:
		size = len(data)
	case 1:
		if len(data) < 16 {
			return 0, 0, "", errors.New("truncated extended mp4 box header")
		}
		extended := binary.BigEndian.Uint64(data[8:16])
		if extended > uint64(len(data)) || extended > uint64(^uint(0)>>1) {
			return 0, 0, "", errors.New("invalid extended mp4 box size")
		}
		size = int(extended)
		headerSize = 16
	default:
		size = int(rawSize)
	}
	if size < headerSize {
		return 0, 0, "", errors.New("invalid mp4 box header")
	}
	return size, headerSize, boxType, nil
}

func parseMP4MovieHeader(payload []byte) (float64, error) {
	if len(payload) < 20 {
		return 0, errors.New("truncated mp4 mvhd box")
	}
	var timescale uint32
	var duration uint64
	switch payload[0] {
	case 0:
		timescale = binary.BigEndian.Uint32(payload[12:16])
		duration = uint64(binary.BigEndian.Uint32(payload[16:20]))
	case 1:
		if len(payload) < 32 {
			return 0, errors.New("truncated version 1 mp4 mvhd box")
		}
		timescale = binary.BigEndian.Uint32(payload[20:24])
		duration = binary.BigEndian.Uint64(payload[24:32])
	default:
		return 0, errors.New("unsupported mp4 mvhd version")
	}
	if timescale == 0 || duration == 0 {
		return 0, errVideoDurationNotFound
	}
	return float64(duration) / float64(timescale), nil
}

const (
	webmSegmentID       = 0x18538067
	webmInfoID          = 0x1549A966
	webmTimecodeScaleID = 0x2AD7B1
	webmDurationID      = 0x4489
)

// parseWebMDurationSeconds reads Matroska/WebM Segment Info. Duration is stored
// in timecode units and TimecodeScale defaults to one millisecond.
func parseWebMDurationSeconds(data []byte) (int, error) {
	duration, err := findWebMDuration(data)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, errVideoDurationNotFound
	}
	seconds := int(math.Ceil(duration))
	if seconds <= 0 {
		return 0, errVideoDurationNotFound
	}
	return seconds, nil
}

func findWebMDuration(data []byte) (float64, error) {
	for offset := 0; offset < len(data); {
		id, idLength, _, err := readEBMLVInt(data[offset:], true)
		if err != nil {
			return 0, err
		}
		size, sizeLength, unknownSize, err := readEBMLVInt(data[offset+idLength:], false)
		if err != nil {
			return 0, err
		}
		payloadStart := offset + idLength + sizeLength
		payloadEnd := payloadStart + int(size)
		if unknownSize {
			payloadEnd = len(data)
		}
		if payloadStart > len(data) || payloadEnd < payloadStart || payloadEnd > len(data) {
			return 0, errors.New("invalid webm element size")
		}
		payload := data[payloadStart:payloadEnd]
		switch id {
		case webmSegmentID:
			if duration, nestedErr := findWebMDuration(payload); nestedErr == nil {
				return duration, nil
			}
		case webmInfoID:
			return parseWebMInfo(payload)
		}
		if unknownSize {
			break
		}
		offset = payloadEnd
	}
	return 0, errVideoDurationNotFound
}

func parseWebMInfo(data []byte) (float64, error) {
	timecodeScale := uint64(1_000_000)
	duration := float64(0)
	for offset := 0; offset < len(data); {
		id, idLength, _, err := readEBMLVInt(data[offset:], true)
		if err != nil {
			return 0, err
		}
		size, sizeLength, unknownSize, err := readEBMLVInt(data[offset+idLength:], false)
		if err != nil || unknownSize {
			return 0, errors.New("invalid webm info element")
		}
		payloadStart := offset + idLength + sizeLength
		payloadEnd := payloadStart + int(size)
		if payloadStart > len(data) || payloadEnd < payloadStart || payloadEnd > len(data) {
			return 0, errors.New("invalid webm info element size")
		}
		payload := data[payloadStart:payloadEnd]
		switch id {
		case webmTimecodeScaleID:
			if len(payload) == 0 || len(payload) > 8 {
				return 0, errors.New("invalid webm timecode scale")
			}
			timecodeScale = 0
			for _, value := range payload {
				timecodeScale = timecodeScale<<8 | uint64(value)
			}
		case webmDurationID:
			switch len(payload) {
			case 4:
				duration = float64(math.Float32frombits(binary.BigEndian.Uint32(payload)))
			case 8:
				duration = math.Float64frombits(binary.BigEndian.Uint64(payload))
			default:
				return 0, errors.New("invalid webm duration")
			}
		}
		offset = payloadEnd
	}
	if timecodeScale == 0 || duration <= 0 {
		return 0, errVideoDurationNotFound
	}
	return duration * float64(timecodeScale) / 1_000_000_000, nil
}

func readEBMLVInt(data []byte, preserveMarker bool) (value uint64, length int, unknown bool, err error) {
	if len(data) == 0 || data[0] == 0 {
		return 0, 0, false, errors.New("invalid ebml variable integer")
	}
	marker := byte(0x80)
	length = 1
	for length <= 8 && data[0]&marker == 0 {
		marker >>= 1
		length++
	}
	if length > 8 || len(data) < length {
		return 0, 0, false, errors.New("truncated ebml variable integer")
	}
	value = uint64(data[0])
	if !preserveMarker {
		value = uint64(data[0] &^ marker)
	}
	for index := 1; index < length; index++ {
		value = value<<8 | uint64(data[index])
	}
	if !preserveMarker {
		unknown = value == (uint64(1)<<(7*length))-1
	}
	return value, length, unknown, nil
}
