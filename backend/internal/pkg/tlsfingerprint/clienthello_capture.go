package tlsfingerprint

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	tlsRecordTypeHandshake  = 22
	tlsHandshakeClientHello = 1
)

// CapturedClientHello 保存从原始 ClientHello 中提取的模板字段。
type CapturedClientHello struct {
	CipherSuites        []uint16
	Curves              []uint16
	PointFormats        []uint16
	SignatureAlgorithms []uint16
	ALPNProtocols       []string
	SupportedVersions   []uint16
	KeyShareGroups      []uint16
	PSKModes            []uint16
	Extensions          []uint16
	EnableGREASE        bool
	JA3Raw              string
	JA3Hash             string
}

// ParseCapturedClientHello 从 TLS 记录字节中解析 ClientHello。
func ParseCapturedClientHello(record []byte) (*CapturedClientHello, error) {
	if len(record) < 9 {
		return nil, errors.New("client hello record too short")
	}
	if record[0] != tlsRecordTypeHandshake {
		return nil, fmt.Errorf("unexpected TLS record type %d", record[0])
	}
	recordLen := int(record[3])<<8 | int(record[4])
	if recordLen <= 0 || len(record) < 5+recordLen {
		return nil, errors.New("incomplete TLS record payload")
	}
	body := record[5 : 5+recordLen]
	if len(body) < 4 || body[0] != tlsHandshakeClientHello {
		return nil, errors.New("TLS record does not contain ClientHello")
	}
	helloLen := int(body[1])<<16 | int(body[2])<<8 | int(body[3])
	if helloLen <= 0 || len(body) < 4+helloLen {
		return nil, errors.New("incomplete ClientHello payload")
	}
	return parseClientHelloBody(body[4 : 4+helloLen])
}

func parseClientHelloBody(data []byte) (*CapturedClientHello, error) {
	offset := 0
	legacyVersion, ok := readUint16(data, &offset)
	if !ok {
		return nil, errors.New("missing legacy version")
	}
	if !skip(data, &offset, 32) {
		return nil, errors.New("missing random")
	}
	sessionIDLen, ok := readUint8(data, &offset)
	if !ok || !skip(data, &offset, int(sessionIDLen)) {
		return nil, errors.New("invalid session id")
	}

	cipherSuitesLen, ok := readUint16(data, &offset)
	if !ok || int(cipherSuitesLen)%2 != 0 || offset+int(cipherSuitesLen) > len(data) {
		return nil, errors.New("invalid cipher suites")
	}
	cipherSuitesRaw := readUint16List(data[offset : offset+int(cipherSuitesLen)])
	offset += int(cipherSuitesLen)

	compressionLen, ok := readUint8(data, &offset)
	if !ok || !skip(data, &offset, int(compressionLen)) {
		return nil, errors.New("invalid compression methods")
	}

	var extensionIDsRaw []uint16
	var curvesRaw []uint16
	var pointFormats []uint16
	var signatureAlgorithms []uint16
	var alpnProtocols []string
	var supportedVersionsRaw []uint16
	var keyShareGroupsRaw []uint16
	var pskModes []uint16

	if offset < len(data) {
		extensionsLen, ok := readUint16(data, &offset)
		if !ok || offset+int(extensionsLen) > len(data) {
			return nil, errors.New("invalid extensions block")
		}
		end := offset + int(extensionsLen)
		for offset < end {
			extType, ok := readUint16(data, &offset)
			if !ok {
				return nil, errors.New("invalid extension type")
			}
			extLen, ok := readUint16(data, &offset)
			if !ok || offset+int(extLen) > end {
				return nil, errors.New("invalid extension payload")
			}
			extData := data[offset : offset+int(extLen)]
			offset += int(extLen)
			extensionIDsRaw = append(extensionIDsRaw, extType)
			switch extType {
			case 10:
				curvesRaw = parseSupportedGroups(extData)
			case 11:
				pointFormats = parsePointFormats(extData)
			case 13:
				signatureAlgorithms = parseUint16Vector(extData)
			case 16:
				alpnProtocols = parseALPN(extData)
			case 43:
				supportedVersionsRaw = parseSupportedVersions(extData)
			case 45:
				pskModes = parsePSKModes(extData)
			case 51:
				keyShareGroupsRaw = parseKeyShareGroups(extData)
			}
		}
	}

	enableGREASE := containsGREASE(cipherSuitesRaw) ||
		containsGREASE(extensionIDsRaw) ||
		containsGREASE(curvesRaw) ||
		containsGREASE(supportedVersionsRaw) ||
		containsGREASE(keyShareGroupsRaw)
	captured := &CapturedClientHello{
		CipherSuites:        normalizeGREASEList(cipherSuitesRaw),
		Curves:              normalizeGREASEList(curvesRaw),
		PointFormats:        pointFormats,
		SignatureAlgorithms: signatureAlgorithms,
		ALPNProtocols:       alpnProtocols,
		SupportedVersions:   normalizeGREASEList(supportedVersionsRaw),
		KeyShareGroups:      normalizeGREASEList(keyShareGroupsRaw),
		PSKModes:            pskModes,
		Extensions:          normalizeGREASEList(extensionIDsRaw),
		EnableGREASE:        enableGREASE,
	}
	captured.JA3Raw = buildJA3Raw(legacyVersion, cipherSuitesRaw, extensionIDsRaw, curvesRaw, pointFormats)
	sum := md5.Sum([]byte(captured.JA3Raw))
	captured.JA3Hash = hex.EncodeToString(sum[:])
	return captured, nil
}

func parseSupportedGroups(data []byte) []uint16 {
	return parseUint16Vector(data)
}

func parsePointFormats(data []byte) []uint16 {
	if len(data) < 1 {
		return nil
	}
	count := int(data[0])
	if 1+count > len(data) {
		return nil
	}
	out := make([]uint16, 0, count)
	for _, v := range data[1 : 1+count] {
		out = append(out, uint16(v))
	}
	return out
}

func parseALPN(data []byte) []string {
	if len(data) < 2 {
		return nil
	}
	listLen := int(data[0])<<8 | int(data[1])
	if 2+listLen > len(data) {
		return nil
	}
	offset := 2
	end := 2 + listLen
	out := make([]string, 0, 2)
	for offset < end {
		nameLen := int(data[offset])
		offset++
		if nameLen == 0 || offset+nameLen > end {
			return out
		}
		out = append(out, string(data[offset:offset+nameLen]))
		offset += nameLen
	}
	return out
}

func parseSupportedVersions(data []byte) []uint16 {
	if len(data) < 1 {
		return nil
	}
	vecLen := int(data[0])
	if 1+vecLen > len(data) || vecLen%2 != 0 {
		return nil
	}
	return readUint16List(data[1 : 1+vecLen])
}

func parsePSKModes(data []byte) []uint16 {
	if len(data) < 1 {
		return nil
	}
	count := int(data[0])
	if 1+count > len(data) {
		return nil
	}
	out := make([]uint16, 0, count)
	for _, v := range data[1 : 1+count] {
		out = append(out, uint16(v))
	}
	return out
}

func parseKeyShareGroups(data []byte) []uint16 {
	if len(data) < 2 {
		return nil
	}
	vecLen := int(data[0])<<8 | int(data[1])
	if 2+vecLen > len(data) {
		return nil
	}
	offset := 2
	end := 2 + vecLen
	out := make([]uint16, 0, 2)
	for offset < end {
		group, ok := readUint16(data, &offset)
		if !ok {
			return out
		}
		keyLen, ok := readUint16(data, &offset)
		if !ok || offset+int(keyLen) > end {
			return out
		}
		out = append(out, group)
		offset += int(keyLen)
	}
	return out
}

func parseUint16Vector(data []byte) []uint16 {
	if len(data) < 2 {
		return nil
	}
	vecLen := int(data[0])<<8 | int(data[1])
	if 2+vecLen > len(data) || vecLen%2 != 0 {
		return nil
	}
	return readUint16List(data[2 : 2+vecLen])
}

func readUint16List(data []byte) []uint16 {
	out := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		out = append(out, uint16(data[i])<<8|uint16(data[i+1]))
	}
	return out
}

func buildJA3Raw(version uint16, cipherSuites, extensions, curves, pointFormats []uint16) string {
	return strings.Join([]string{
		strconv.Itoa(int(version)),
		joinJA3Values(cipherSuites),
		joinJA3Values(extensions),
		joinJA3Values(curves),
		joinJA3Values(pointFormats),
	}, ",")
}

func joinJA3Values(values []uint16) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		if isGREASEValue(v) {
			continue
		}
		parts = append(parts, strconv.Itoa(int(v)))
	}
	return strings.Join(parts, "-")
}

func normalizeGREASEList(values []uint16) []uint16 {
	if len(values) == 0 {
		return nil
	}
	out := make([]uint16, 0, len(values))
	for _, v := range values {
		if isGREASEValue(v) {
			out = append(out, 0x0a0a)
			continue
		}
		out = append(out, v)
	}
	return out
}

func containsGREASE(values []uint16) bool {
	for _, v := range values {
		if isGREASEValue(v) {
			return true
		}
	}
	return false
}

func readUint8(data []byte, offset *int) (uint8, bool) {
	if *offset >= len(data) {
		return 0, false
	}
	v := data[*offset]
	*offset += 1
	return v, true
}

func readUint16(data []byte, offset *int) (uint16, bool) {
	if *offset+2 > len(data) {
		return 0, false
	}
	v := uint16(data[*offset])<<8 | uint16(data[*offset+1])
	*offset += 2
	return v, true
}

func skip(data []byte, offset *int, n int) bool {
	if n < 0 || *offset+n > len(data) {
		return false
	}
	*offset += n
	return true
}
