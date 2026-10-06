package muse

// The wire field numbers and framing limits were observed in the consumer
// Muse client on 2026-10-06. This is a native protocol implementation, not an
// embedded browser client. See docs/MUSE_NATIVE_PROTOCOL.md.

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/flynn/noise"
	"google.golang.org/protobuf/encoding/protowire"
)

var ErrNoiseProtocol = errors.New("invalid Muse encrypted transport frame")

const (
	noiseMaxFrame         = 65535
	noiseChunkSize        = 65489
	noiseMaxChunks        = 256
	noiseMaxAssemblyBytes = 16 << 20
	noiseMaxAssemblies    = 16
	noiseMaxNonce         = 1<<53 - 1
)

type NoiseHeader struct{ Key, Value string }

// NoiseFrame represents an HTTP response or body chunk on a multiplexed
// stream. A reset terminates that stream, not the remote agent task.
type NoiseFrame struct {
	StreamID    int64
	Kind        string
	Status      int
	Headers     []NoiseHeader
	Body        []byte
	EndBody     bool
	ResetCode   int
	ResetReason string
}

type noiseAssembly struct {
	chunks  map[uint64][]byte
	total   uint64
	bytes   int
	created time.Time
}

// NoiseTransport serializes nonce advancement and poisons both directions on
// any framing/authentication failure. Retrying a frame on a damaged channel
// would desynchronize its implicit nonces.
type NoiseTransport struct {
	mu            sync.Mutex
	send, receive *noise.CipherState
	nextStream    int64
	dead          bool
	assemblies    map[uint64]*noiseAssembly
	retained      int
	now           func() time.Time
}

func newNoiseTransport(send, receive *noise.CipherState) *NoiseTransport {
	return &NoiseTransport{send: send, receive: receive, nextStream: 1, assemblies: make(map[uint64]*noiseAssembly), now: time.Now}
}

// EncryptRequest returns all encrypted frames in send order. The caller must
// send that entire batch before another request's frames. Service 0 is daemon;
// the other observed services (Sentinel, vault, authd) are not exposed here.
func (t *NoiseTransport) EncryptRequest(method, path string, headers []NoiseHeader, body []byte) (int64, [][]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dead || t.nextStream <= 0 || t.nextStream == math.MaxInt64 || len(body) > noiseMaxAssemblyBytes || len(method) > 16 || len(path) > 4096 || len(headers) > 128 || !utf8.ValidString(method) || !utf8.ValidString(path) {
		return 0, nil, t.fail()
	}
	id := t.nextStream
	t.nextStream++
	r := noiseBytes(nil, 1, []byte(method))
	r = noiseBytes(r, 2, []byte(path))
	for _, header := range headers {
		if len(header.Key)+len(header.Value) > 8192 || !utf8.ValidString(header.Key) || !utf8.ValidString(header.Value) {
			return 0, nil, t.fail()
		}
		h := noiseBytes(nil, 1, []byte(header.Key))
		h = noiseBytes(h, 2, []byte(header.Value))
		r = noiseBytes(r, 3, h)
	}
	r = noiseBytes(r, 4, body)
	r = noiseUint(r, 5, 1)
	frame := noiseUint(nil, 1, uint64(id))
	frame = noiseBytes(frame, 2, r)
	// ServiceRequest.service defaults to daemon (0).
	frames, err := t.encrypt(noiseBytes(nil, 2, frame))
	return id, frames, err
}

// EncryptReset cancels only the transport stream. Provider.Cancel must still
// obtain confirmation of the original agent task's terminal state.
func (t *NoiseTransport) EncryptReset(streamID int64) ([][]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dead || streamID <= 0 {
		return nil, t.fail()
	}
	reset := noiseUint(nil, 1, 1) // CANCELLED
	frame := noiseUint(nil, 1, uint64(streamID))
	return t.encrypt(noiseBytes(nil, 2, noiseBytes(frame, 5, reset)))
}

func (t *NoiseTransport) encrypt(payload []byte) ([][]byte, error) {
	if len(payload) > noiseMaxAssemblyBytes {
		return nil, t.fail()
	}
	var randomID [8]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return nil, t.fail()
	}
	id := binary.LittleEndian.Uint64(randomID[:])
	count := (len(payload) + noiseChunkSize - 1) / noiseChunkSize
	if count == 0 {
		count = 1
	}
	if count > noiseMaxChunks {
		return nil, t.fail()
	}
	frames := make([][]byte, 0, count)
	for i := range count {
		end := min((i+1)*noiseChunkSize, len(payload))
		chunk := noiseUint(nil, 1, id)
		chunk = noiseUint(chunk, 2, uint64(i))
		chunk = noiseUint(chunk, 3, uint64(count))
		chunk = noiseBytes(chunk, 4, payload[i*noiseChunkSize:end])
		if t.send == nil || t.send.Nonce() >= noiseMaxNonce {
			return nil, t.fail()
		}
		encrypted, err := t.send.Encrypt(nil, nil, chunk)
		if err != nil || len(encrypted) > noiseMaxFrame {
			return nil, t.fail()
		}
		frames = append(frames, encrypted)
	}
	return frames, nil
}

// DecryptFrame returns nil while a bounded multi-frame message is incomplete.
func (t *NoiseTransport) DecryptFrame(frame []byte) (*NoiseFrame, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dead || len(frame) < 16 || len(frame) > noiseMaxFrame || t.receive == nil || t.receive.Nonce() >= noiseMaxNonce {
		return nil, t.fail()
	}
	plain, err := t.receive.Decrypt(nil, nil, frame)
	if err != nil {
		return nil, t.fail()
	}
	var id, index uint64
	total := uint64(1)
	var payload []byte
	seen := map[protowire.Number]bool{}
	err = noiseFields(plain, func(n protowire.Number, kind protowire.Type, value []byte) error {
		if n < 1 || n > 4 {
			return nil
		}
		if seen[n] {
			return ErrNoiseProtocol
		}
		seen[n] = true
		if n == 4 {
			payload, err = noiseReadBytes(kind, value)
			return err
		}
		v, e := noiseReadUint(kind, value)
		if e != nil {
			return e
		}
		switch n {
		case 1:
			id = v
		case 2:
			index = v
		case 3:
			total = v
		}
		return nil
	})
	if err != nil || total < 1 || total > noiseMaxChunks || index >= total || len(payload) > noiseChunkSize {
		return nil, t.fail()
	}
	for key, assembly := range t.assemblies {
		if t.now().Sub(assembly.created) > time.Minute {
			t.retained -= assembly.bytes
			delete(t.assemblies, key)
		}
	}
	a := t.assemblies[id]
	if a == nil {
		if len(t.assemblies) >= noiseMaxAssemblies {
			return nil, t.fail()
		}
		a = &noiseAssembly{chunks: make(map[uint64][]byte), total: total, created: t.now()}
		t.assemblies[id] = a
	}
	if a.total != total || a.chunks[index] != nil || t.retained+len(payload) > noiseMaxAssemblyBytes {
		return nil, t.fail()
	}
	a.chunks[index] = append([]byte{}, payload...)
	a.bytes += len(payload)
	t.retained += len(payload)
	if uint64(len(a.chunks)) != total {
		return nil, nil
	}
	assembled := make([]byte, 0, a.bytes)
	for i := uint64(0); i < total; i++ {
		assembled = append(assembled, a.chunks[i]...)
	}
	t.retained -= a.bytes
	delete(t.assemblies, id)
	var envelope []byte
	err = noiseFields(assembled, func(n protowire.Number, kind protowire.Type, value []byte) error {
		if n != 1 {
			return nil
		}
		if envelope != nil {
			return ErrNoiseProtocol
		}
		envelope, err = noiseReadBytes(kind, value)
		return err
	})
	if err != nil || len(envelope) == 0 {
		return nil, t.fail()
	}
	result, err := decodeNoiseEnvelope(envelope)
	if err != nil {
		return nil, t.fail()
	}
	return result, nil
}

func (t *NoiseTransport) fail() error {
	t.dead = true
	clear(t.assemblies)
	t.retained = 0
	return ErrNoiseProtocol
}

func decodeNoiseEnvelope(data []byte) (*NoiseFrame, error) {
	frame := &NoiseFrame{}
	var body []byte
	seen := map[protowire.Number]bool{}
	err := noiseFields(data, func(n protowire.Number, kind protowire.Type, value []byte) error {
		if n < 1 || n > 5 {
			return nil
		}
		if seen[n] {
			return ErrNoiseProtocol
		}
		seen[n] = true
		if n == 1 {
			v, e := noiseReadUint(kind, value)
			frame.StreamID = int64(v)
			return e
		}
		if frame.Kind != "" {
			return ErrNoiseProtocol
		}
		switch n {
		case 3:
			frame.Kind = "response"
		case 4:
			frame.Kind = "body_chunk"
		case 5:
			frame.Kind = "reset"
		default:
			return ErrNoiseProtocol
		}
		var e error
		body, e = noiseReadBytes(kind, value)
		return e
	})
	if err != nil || frame.StreamID <= 0 || frame.Kind == "" {
		return nil, ErrNoiseProtocol
	}
	seen = map[protowire.Number]bool{}
	err = noiseFields(body, func(n protowire.Number, kind protowire.Type, value []byte) error {
		if n < 1 || n > 4 {
			return nil
		}
		if frame.Kind != "response" || n != 2 {
			if seen[n] {
				return ErrNoiseProtocol
			}
			seen[n] = true
		}
		if frame.Kind == "response" {
			switch n {
			case 1:
				v, e := noiseReadUint(kind, value)
				if v < 100 || v > 599 {
					return ErrNoiseProtocol
				}
				frame.Status = int(v)
				return e
			case 2:
				h, e := noiseReadBytes(kind, value)
				if e != nil || len(frame.Headers) >= 128 {
					return ErrNoiseProtocol
				}
				header, e := decodeNoiseHeader(h)
				frame.Headers = append(frame.Headers, header)
				return e
			case 3:
				b, e := noiseReadBytes(kind, value)
				frame.Body = b
				return e
			case 4:
				v, e := noiseReadUint(kind, value)
				if v > 1 {
					return ErrNoiseProtocol
				}
				frame.EndBody = v == 1
				return e
			}
		}
		if frame.Kind == "body_chunk" {
			switch n {
			case 1:
				b, e := noiseReadBytes(kind, value)
				frame.Body = b
				return e
			case 2:
				v, e := noiseReadUint(kind, value)
				if v > 1 {
					return ErrNoiseProtocol
				}
				frame.EndBody = v == 1
				return e
			}
		}
		if frame.Kind == "reset" {
			switch n {
			case 1:
				v, e := noiseReadUint(kind, value)
				if v > 6 {
					return ErrNoiseProtocol
				}
				frame.ResetCode = int(v)
				return e
			case 2:
				b, e := noiseReadBytes(kind, value)
				if len(b) > 8192 || !utf8.Valid(b) {
					return ErrNoiseProtocol
				}
				frame.ResetReason = string(b)
				return e
			}
		}
		return nil
	})
	if err != nil || (frame.Kind == "response" && frame.Status == 0) {
		return nil, ErrNoiseProtocol
	}
	return frame, nil
}

func decodeNoiseHeader(data []byte) (NoiseHeader, error) {
	var h NoiseHeader
	seen := map[protowire.Number]bool{}
	err := noiseFields(data, func(n protowire.Number, kind protowire.Type, value []byte) error {
		if n != 1 && n != 2 {
			return nil
		}
		if seen[n] {
			return ErrNoiseProtocol
		}
		seen[n] = true
		b, e := noiseReadBytes(kind, value)
		if n == 1 {
			h.Key = string(b)
		} else {
			h.Value = string(b)
		}
		return e
	})
	if err != nil || len(h.Key)+len(h.Value) > 8192 || !utf8.ValidString(h.Key) || !utf8.ValidString(h.Value) {
		return h, ErrNoiseProtocol
	}
	return h, nil
}

func noiseFields(data []byte, visit func(protowire.Number, protowire.Type, []byte) error) error {
	for len(data) > 0 {
		n, kind, tagBytes := protowire.ConsumeTag(data)
		if tagBytes < 0 || n <= 0 || kind == protowire.StartGroupType || kind == protowire.EndGroupType {
			return ErrNoiseProtocol
		}
		data = data[tagBytes:]
		length := protowire.ConsumeFieldValue(n, kind, data)
		if length < 0 {
			return ErrNoiseProtocol
		}
		if err := visit(n, kind, data[:length]); err != nil {
			return err
		}
		data = data[length:]
	}
	return nil
}
func noiseReadUint(kind protowire.Type, data []byte) (uint64, error) {
	if kind != protowire.VarintType {
		return 0, ErrNoiseProtocol
	}
	v, n := protowire.ConsumeVarint(data)
	if n < 0 {
		return 0, ErrNoiseProtocol
	}
	return v, nil
}
func noiseReadBytes(kind protowire.Type, data []byte) ([]byte, error) {
	if kind != protowire.BytesType {
		return nil, ErrNoiseProtocol
	}
	v, n := protowire.ConsumeBytes(data)
	if n < 0 {
		return nil, ErrNoiseProtocol
	}
	return v, nil
}
func noiseUint(out []byte, n protowire.Number, v uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(out, n, protowire.VarintType), v)
}
func noiseBytes(out []byte, n protowire.Number, v []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(out, n, protowire.BytesType), v)
}
