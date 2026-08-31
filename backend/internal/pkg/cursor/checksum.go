package cursor

import "time"

const checksumAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// GenerateChecksum computes the x-cursor-checksum header value using the Jyh
// cipher: a time-based XOR with rolling key seed 165, base64url-encoded, then
// concatenated with the machine ID.
func GenerateChecksum(machineID string) string {
	return GenerateChecksumAt(machineID, "", time.Now())
}

// GenerateChecksumAt is the time-parameterized variant (for testing).
// If macMachineID is set, Cursor 3.16 sends `{encoded}{machineId}/{macMachineId}`.
func GenerateChecksumAt(machineID, macMachineID string, now time.Time) string {
	// Coarse timestamp: milliseconds / 1,000,000 ≈ ~1000-second units.
	ts := now.UnixMilli() / 1_000_000

	byteArray := [6]byte{
		byte((ts >> 40) & 0xFF),
		byte((ts >> 32) & 0xFF),
		byte((ts >> 24) & 0xFF),
		byte((ts >> 16) & 0xFF),
		byte((ts >> 8) & 0xFF),
		byte(ts & 0xFF),
	}

	// Jyh cipher: XOR with rolling key, add positional offset.
	t := byte(165)
	for i := 0; i < len(byteArray); i++ {
		byteArray[i] = ((byteArray[i] ^ t) + byte(i%256)) & 0xFF
		t = byteArray[i]
	}

	// Custom base64url encoding (no padding).
	var encoded []byte
	for i := 0; i < len(byteArray); i += 3 {
		a := byteArray[i]
		var b, c byte
		if i+1 < len(byteArray) {
			b = byteArray[i+1]
		}
		if i+2 < len(byteArray) {
			c = byteArray[i+2]
		}

		encoded = append(encoded, checksumAlphabet[a>>2])
		encoded = append(encoded, checksumAlphabet[((a&3)<<4)|(b>>4)])

		if i+1 < len(byteArray) {
			encoded = append(encoded, checksumAlphabet[((b&15)<<2)|(c>>6)])
		}
		if i+2 < len(byteArray) {
			encoded = append(encoded, checksumAlphabet[c&63])
		}
	}

	out := string(encoded) + machineID
	if macMachineID != "" {
		out += "/" + macMachineID
	}
	return out
}
