//go:build unit

package muse

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/flynn/noise"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

type noiseTestSocket struct {
	responder     *noise.HandshakeState
	secondPayload []byte
	second        []byte
	receive, send *noise.CipherState
	writes        int
}

func newNoiseTestSocket(t *testing.T, payload []byte) *noiseTestSocket {
	t.Helper()
	suite := noise.NewCipherSuite(noise.DH25519, noise.CipherAESGCM, noise.HashSHA256)
	static, err := suite.GenerateKeypair(rand.Reader)
	require.NoError(t, err)
	hs, err := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeXX, StaticKeypair: static})
	require.NoError(t, err)
	return &noiseTestSocket{responder: hs, secondPayload: payload}
}
func (s *noiseTestSocket) Write(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.writes++
	if s.writes == 1 {
		first, _, _, err := s.responder.ReadMessage(nil, data)
		if err != nil {
			return err
		}
		// The browser sends a protobuf client nonce, not an empty payload.
		if len(first) != 34 || first[0] != 10 || first[1] != 32 {
			return ErrNoiseProtocol
		}
		s.second, _, _, err = s.responder.WriteMessage(nil, s.secondPayload)
		return err
	}
	_, receive, send, err := s.responder.ReadMessage(nil, data)
	s.receive, s.send = receive, send
	return err
}
func (s *noiseTestSocket) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.second, nil
}

func testNoiseTransport(t *testing.T) (*NoiseTransport, *noiseTestSocket) {
	t.Helper()
	socket := newNoiseTestSocket(t, nil)
	wire, err := HandshakeNoise(context.Background(), socket, func(_ context.Context, peer NoisePeer) error {
		require.Len(t, peer.StaticKey, 32)
		require.Len(t, peer.HandshakeHash, 32)
		require.Len(t, peer.ClientNonce, 32)
		return nil
	})
	require.NoError(t, err)
	return wire, socket
}

func encryptTestChunk(t *testing.T, socket *noiseTestSocket, id, index, total uint64, payload []byte) []byte {
	t.Helper()
	chunk := noiseUint(nil, 1, id)
	chunk = noiseUint(chunk, 2, index)
	chunk = noiseUint(chunk, 3, total)
	chunk = noiseBytes(chunk, 4, payload)
	data, err := socket.send.Encrypt(nil, nil, chunk)
	require.NoError(t, err)
	return data
}

func TestNoiseHandshakeRequiresTrustBeforeMessage3(t *testing.T) {
	challenge := noiseBytes(nil, 1, []byte("synthetic attestation"))
	challenge = noiseBytes(challenge, 3, make([]byte, 32))
	unprovisioned := noiseUint(append([]byte(nil), challenge...), 2, 1)
	provisioned := noiseBytes(noiseUint(append([]byte(nil), challenge...), 2, 2), 4, make([]byte, 32))
	for _, test := range []struct {
		name    string
		payload []byte
		trust   NoiseTrust
	}{
		{"missing trust", nil, nil},
		{"rejected peer", nil, func(context.Context, NoisePeer) error { return errors.New("rejected") }},
		{"unprovisioned confidential peer", unprovisioned, func(context.Context, NoisePeer) error { t.Fatal("must not disclose credentials"); return nil }},
		{"provisioned confidential peer", provisioned, func(context.Context, NoisePeer) error { t.Fatal("must not ignore owner RV"); return nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			socket := newNoiseTestSocket(t, test.payload)
			_, err := HandshakeNoise(context.Background(), socket, test.trust)
			require.ErrorIs(t, err, ErrNoiseTrust)
			require.Less(t, socket.writes, 2)
		})
	}
}

func TestNoiseNativeRequestAndMultiplexedResponse(t *testing.T) {
	wire, socket := testNoiseTransport(t)
	stream, outgoing, err := wire.EncryptRequest("GET", "/model", []NoiseHeader{{Key: "x-app-id", Value: "hatch-web"}}, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, stream)
	require.Len(t, outgoing, 1)
	plain, err := socket.receive.Decrypt(nil, nil, outgoing[0])
	require.NoError(t, err)
	// Verify the request carries the observed application path and header,
	// without plaintext appearing in its encrypted WebSocket frame.
	require.Contains(t, string(plain), "/model")
	require.Contains(t, string(plain), "hatch-web")
	require.NotContains(t, string(outgoing[0]), "/model")
	// Independent fixed wire fixture: ServiceResponse{payload: ServiceFrame{
	// stream_id:1,response:{status:200,body:"{}",end_body:true}}}.
	fixture := []byte{0x0a, 0x0d, 0x08, 0x01, 0x1a, 0x09, 0x08, 0xc8, 0x01, 0x1a, 0x02, 0x7b, 0x7d, 0x20, 0x01}
	response, err := wire.DecryptFrame(encryptTestChunk(t, socket, 17, 0, 1, fixture))
	require.NoError(t, err)
	require.Equal(t, &NoiseFrame{StreamID: 1, Kind: "response", Status: 200, Body: []byte("{}"), EndBody: true}, response)
	stream, _, err = wire.EncryptRequest("GET", "/model", nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 2, stream)
}

func TestNoiseChunkAssemblyAndFailurePoisoning(t *testing.T) {
	wire, socket := testNoiseTransport(t)
	body := bytes.Repeat([]byte("x"), noiseChunkSize+100)
	envelope := noiseUint(nil, 1, 4)
	envelope = noiseBytes(envelope, 4, noiseBytes(nil, 1, body))
	payload := noiseBytes(nil, 1, envelope)
	second := payload[noiseChunkSize:]
	result, err := wire.DecryptFrame(encryptTestChunk(t, socket, 19, 1, 2, second))
	require.NoError(t, err)
	require.Nil(t, result)
	result, err = wire.DecryptFrame(encryptTestChunk(t, socket, 19, 0, 2, payload[:noiseChunkSize]))
	require.NoError(t, err)
	require.EqualValues(t, 4, result.StreamID)
	require.Equal(t, body, result.Body)
	require.Zero(t, wire.retained)
	_, err = wire.DecryptFrame(encryptTestChunk(t, socket, 20, 0, 2, []byte("part")))
	require.NoError(t, err)
	_, err = wire.DecryptFrame(encryptTestChunk(t, socket, 20, 0, 2, []byte("duplicate")))
	require.ErrorIs(t, err, ErrNoiseProtocol)
	_, _, err = wire.EncryptRequest("GET", "/model", nil, nil)
	require.ErrorIs(t, err, ErrNoiseProtocol)
	require.Zero(t, wire.retained)
}

func TestNoiseRejectsMalformedAndUnauthenticatedFrames(t *testing.T) {
	for _, test := range []struct {
		name  string
		frame func(*testing.T, *noiseTestSocket) []byte
	}{
		{"invalid GCM tag", func(_ *testing.T, _ *noiseTestSocket) []byte { return make([]byte, 32) }},
		{"oversized WebSocket", func(_ *testing.T, _ *noiseTestSocket) []byte { return make([]byte, noiseMaxFrame+1) }},
		{"zero chunks", func(t *testing.T, s *noiseTestSocket) []byte { return encryptTestChunk(t, s, 1, 0, 0, nil) }},
		{"too many chunks", func(t *testing.T, s *noiseTestSocket) []byte { return encryptTestChunk(t, s, 1, 0, 257, nil) }},
		{"index out of range", func(t *testing.T, s *noiseTestSocket) []byte { return encryptTestChunk(t, s, 1, 1, 1, nil) }},
		{"truncated protobuf", func(t *testing.T, s *noiseTestSocket) []byte { return encryptTestChunk(t, s, 1, 0, 1, []byte{10, 100}) }},
		{"request from server", func(t *testing.T, s *noiseTestSocket) []byte {
			e := noiseBytes(noiseUint(nil, 1, 1), 2, nil)
			return encryptTestChunk(t, s, 1, 0, 1, noiseBytes(nil, 1, e))
		}},
		{"duplicate stream ID", func(t *testing.T, s *noiseTestSocket) []byte {
			e := noiseUint(noiseUint(nil, 1, 1), 1, 2)
			e = noiseBytes(e, 4, nil)
			return encryptTestChunk(t, s, 1, 0, 1, noiseBytes(nil, 1, e))
		}},
		{"multiple envelope kinds", func(t *testing.T, s *noiseTestSocket) []byte {
			e := noiseBytes(noiseBytes(noiseUint(nil, 1, 1), 4, nil), 5, nil)
			return encryptTestChunk(t, s, 1, 0, 1, noiseBytes(nil, 1, e))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire, socket := testNoiseTransport(t)
			_, err := wire.DecryptFrame(test.frame(t, socket))
			require.ErrorIs(t, err, ErrNoiseProtocol)
			_, err = wire.DecryptFrame([]byte{1})
			require.ErrorIs(t, err, ErrNoiseProtocol)
		})
	}
}

func TestNoisePendingAssemblyLimitAndReset(t *testing.T) {
	wire, socket := testNoiseTransport(t)
	frames, err := wire.EncryptReset(12)
	require.NoError(t, err)
	require.Len(t, frames, 1)
	plain, err := socket.receive.Decrypt(nil, nil, frames[0])
	require.NoError(t, err)
	require.NotEmpty(t, plain)
	for i := uint64(0); i < noiseMaxAssemblies; i++ {
		_, err = wire.DecryptFrame(encryptTestChunk(t, socket, i, 0, 2, []byte("part")))
		require.NoError(t, err)
	}
	_, err = wire.DecryptFrame(encryptTestChunk(t, socket, noiseMaxAssemblies, 0, 2, nil))
	require.ErrorIs(t, err, ErrNoiseProtocol)
}

func FuzzNoiseEnvelope(f *testing.F) {
	f.Add([]byte{8, 1, 34, 0})
	f.Add([]byte{8, 1, 26, 3, 8, 200, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _ = decodeNoiseEnvelope(data)
	})
}

func TestNoiseRejectsProtobufGroups(t *testing.T) {
	data := protowire.AppendTag(nil, 1, protowire.StartGroupType)
	_, err := decodeNoiseEnvelope(data)
	require.ErrorIs(t, err, ErrNoiseProtocol)
}
