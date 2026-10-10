package muse

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/flynn/noise"
	"google.golang.org/protobuf/encoding/protowire"
)

var ErrNoiseTrust = errors.New("muse VM trust has not been qualified")

// NoiseSocket implementations must retain normal TLS validation and use the
// account's configured proxy. Reads and writes are binary WebSocket messages.
type NoiseSocket interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
}

type NoisePeer struct {
	StaticKey     []byte
	HandshakeHash []byte
	ClientNonce   []byte
	Attestation   []byte
}

// NoiseTrust verifies the observed standard peer before sending Message 3.
// There is deliberately no implicit accept-all callback. Confidential owner-RV
// challenges need a separate verified key flow and currently fail closed.
type NoiseTrust func(context.Context, NoisePeer) error

// VerifyStandardNoisePeer validates the observed standard VM statement. It
// binds the Noise static key to Message 1's freshness nonce. This does not
// perform hardware attestation and must never be used for confidential peers.
// The socket caller remains responsible for TLS and authenticated VM routing.
func VerifyStandardNoisePeer(_ context.Context, peer NoisePeer) error {
	if len(peer.StaticKey) != 32 || len(peer.ClientNonce) != 32 || len(peer.HandshakeHash) != 32 {
		return ErrNoiseTrust
	}
	var statement []byte
	err := noiseFields(peer.Attestation, func(n protowire.Number, kind protowire.Type, value []byte) error {
		if n != 2 || statement != nil {
			return ErrNoiseProtocol
		}
		var e error
		statement, e = noiseReadBytes(kind, value)
		return e
	})
	if err != nil || len(statement) == 0 {
		return ErrNoiseTrust
	}
	var key, nonce []byte
	err = noiseFields(statement, func(n protowire.Number, kind protowire.Type, value []byte) error {
		var e error
		switch n {
		case 1:
			if key != nil {
				return ErrNoiseProtocol
			}
			key, e = noiseReadBytes(kind, value)
		case 2:
			if nonce != nil {
				return ErrNoiseProtocol
			}
			nonce, e = noiseReadBytes(kind, value)
		default:
			return ErrNoiseProtocol
		}
		return e
	})
	if err != nil || subtle.ConstantTimeCompare(key, peer.StaticKey) != 1 || subtle.ConstantTimeCompare(nonce, peer.ClientNonce) != 1 {
		return ErrNoiseTrust
	}
	return nil
}

func HandshakeNoise(ctx context.Context, socket NoiseSocket, trust NoiseTrust) (*NoiseTransport, error) {
	if socket == nil || trust == nil {
		return nil, ErrNoiseTrust
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	suite := noise.NewCipherSuite(noise.DH25519, noise.CipherAESGCM, noise.HashSHA256)
	static, err := suite.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, ErrNoiseProtocol
	}
	defer clear(static.Private)
	handshake, err := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeXX, Initiator: true, StaticKeypair: static})
	if err != nil {
		return nil, ErrNoiseProtocol
	}
	nonce := make([]byte, 32)
	if _, err = rand.Read(nonce); err != nil {
		return nil, ErrNoiseProtocol
	}
	first, _, _, err := handshake.WriteMessage(nil, noiseBytes(nil, 1, nonce))
	if err != nil {
		return nil, ErrNoiseProtocol
	}
	if err = socket.Write(ctx, first); err != nil {
		return nil, ErrNoiseProtocol
	}
	second, err := socket.Read(ctx)
	if err != nil || len(second) < 96 || len(second) > noiseMaxFrame {
		return nil, ErrNoiseProtocol
	}
	payload, _, _, err := handshake.ReadMessage(nil, second)
	if err != nil {
		return nil, ErrNoiseProtocol
	}
	if noiseOwnerChallenge(payload) {
		return nil, ErrNoiseTrust
	}
	peer := NoisePeer{StaticKey: append([]byte(nil), handshake.PeerStatic()...), HandshakeHash: append([]byte(nil), handshake.ChannelBinding()...), ClientNonce: nonce, Attestation: payload}
	if len(peer.StaticKey) != 32 || trust(ctx, peer) != nil || ctx.Err() != nil {
		return nil, ErrNoiseTrust
	}
	third, send, receive, err := handshake.WriteMessage(nil, nil)
	if err != nil || send == nil || receive == nil {
		return nil, ErrNoiseProtocol
	}
	if err = socket.Write(ctx, third); err != nil {
		return nil, ErrNoiseProtocol
	}
	return newNoiseTransport(send, receive), nil
}

// Recognize the complete owner-RV envelope. Standard attestation payloads
// also use protobuf field 2, so that field alone cannot identify an RV peer.
// The mandatory trust callback must validate all other attestation payloads.
func noiseOwnerChallenge(payload []byte) bool {
	var state uint64
	var nonce, proof []byte
	seen := map[protowire.Number]bool{}
	err := noiseFields(payload, func(n protowire.Number, kind protowire.Type, value []byte) error {
		if n < 1 || n > 4 {
			return nil
		}
		if seen[n] {
			return ErrNoiseProtocol
		}
		seen[n] = true
		var e error
		switch n {
		case 1:
			_, e = noiseReadBytes(kind, value)
		case 2:
			state, e = noiseReadUint(kind, value)
		case 3:
			nonce, e = noiseReadBytes(kind, value)
		case 4:
			proof, e = noiseReadBytes(kind, value)
		}
		return e
	})
	return err == nil && seen[1] && len(nonce) == 32 && ((state == 1 && !seen[4]) || (state == 2 && len(proof) == 32))
}
