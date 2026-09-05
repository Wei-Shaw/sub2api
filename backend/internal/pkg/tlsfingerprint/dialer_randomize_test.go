package tlsfingerprint

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestRandomizedProfileChangesWireExtensionOrderPerConnection(t *testing.T) {
	want := []uint16{0, 5, 10, 11, 13, 16, 23, 35, 43, 45, 51}
	profile := &Profile{
		Name:                    "wire-randomization",
		Extensions:              append([]uint16(nil), want...),
		ALPNProtocols:           []string{"h2", "http/1.1"},
		RandomizeExtensionOrder: true,
	}
	before := cloneProfileForTest(profile)

	orders := captureClientHelloExtensionOrders(t, profile, 12)
	seen := make(map[string]struct{}, len(orders))
	for i, order := range orders {
		if !sameUint16Multiset(order, want) {
			t.Fatalf("connection %d changed extension set: got %v want %v", i, order, want)
		}
		seen[fmt.Sprint(order)] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("12 forced new connections produced only %d extension order; want at least 2", len(seen))
	}
	if !reflect.DeepEqual(before, profile) {
		t.Fatalf("building ClientHellos mutated the shared profile:\nbefore: %#v\nafter:  %#v", before, profile)
	}
}

func TestShuffleExtensionOrderPinsPreSharedKeyLast(t *testing.T) {
	want := []uint16{41, 0, 5, 10, 11, 13, 16, 23, 35, 43, 45, 51}
	for i := 0; i < 100; i++ {
		got := shuffleExtensionOrder(want)
		if !sameUint16Multiset(got, want) {
			t.Fatalf("iteration %d changed extension set: got %v want %v", i, got, want)
		}
		if got[len(got)-1] != 41 {
			t.Fatalf("iteration %d placed pre_shared_key at index %d: %v", i, indexUint16(got, 41), got)
		}

		spec := buildClientHelloSpecFromProfile(&Profile{
			Extensions:              want,
			RandomizeExtensionOrder: true,
		})
		last, ok := spec.Extensions[len(spec.Extensions)-1].(*utls.GenericExtension)
		if !ok || last.Id != preSharedKeyExtensionID {
			t.Fatalf("iteration %d did not serialize pre_shared_key last: %T %#v", i, spec.Extensions[len(spec.Extensions)-1], spec.Extensions[len(spec.Extensions)-1])
		}
	}
}

func TestExtensionRandomizationChangesV2StableID(t *testing.T) {
	fixed := &Profile{Extensions: []uint16{0, 10, 16, 43}}
	randomized := &Profile{
		Extensions:              []uint16{0, 10, 16, 43},
		RandomizeExtensionOrder: true,
	}

	if !strings.HasPrefix(fixed.StableID(), "v2:") || !strings.HasPrefix(randomized.StableID(), "v2:") {
		t.Fatalf("StableID schema was not bumped to v2: fixed=%q randomized=%q", fixed.StableID(), randomized.StableID())
	}
	if fixed.StableID() == randomized.StableID() {
		t.Fatalf("fixed and randomized profiles aliased to StableID %q", fixed.StableID())
	}
}

func TestFixedProfileKeepsWireExtensionOrder(t *testing.T) {
	want := []uint16{0, 5, 10, 11, 13, 16, 23, 35, 43, 45, 51}
	profile := &Profile{
		Name:          "wire-fixed-order",
		Extensions:    append([]uint16(nil), want...),
		ALPNProtocols: []string{"http/1.1"},
	}

	orders := captureClientHelloExtensionOrders(t, profile, 2)
	for i, got := range orders {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("connection %d changed fixed extension order: got %v want %v", i, got, want)
		}
	}
}

func captureClientHelloExtensionOrders(t *testing.T, profile *Profile, count int) [][]uint16 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	captured := make(chan []uint16, count)
	serverErr := make(chan error, 1)
	go func() {
		for i := 0; i < count; i++ {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				serverErr <- acceptErr
				return
			}
			order, readErr := readClientHelloExtensionOrder(conn)
			_ = conn.Close()
			if readErr != nil {
				serverErr <- readErr
				return
			}
			captured <- order
		}
		serverErr <- nil
	}()

	dialer := NewDialer(profile, func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
	})
	for i := 0; i < count; i++ {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		conn, dialErr := dialer.DialTLSContext(ctx, "tcp", "capture.example:443")
		cancel()
		if conn != nil {
			_ = conn.Close()
		}
		if dialErr == nil {
			t.Fatalf("connection %d unexpectedly completed a TLS handshake against the capture-only listener", i)
		}
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("capture ClientHello: %v", err)
	}
	orders := make([][]uint16, 0, count)
	for i := 0; i < count; i++ {
		orders = append(orders, <-captured)
	}
	return orders
}

func readClientHelloExtensionOrder(conn net.Conn) ([]uint16, error) {
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return nil, err
	}
	var handshake []byte
	for {
		var header [5]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return nil, err
		}
		if header[0] != 22 {
			return nil, fmt.Errorf("unexpected TLS record type %d", header[0])
		}
		record := make([]byte, int(binary.BigEndian.Uint16(header[3:5])))
		if _, err := io.ReadFull(conn, record); err != nil {
			return nil, err
		}
		handshake = append(handshake, record...)
		if len(handshake) < 4 {
			continue
		}
		if handshake[0] != 1 {
			return nil, fmt.Errorf("unexpected handshake type %d", handshake[0])
		}
		messageLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
		if len(handshake) >= 4+messageLen {
			return parseClientHelloExtensionOrder(handshake[4 : 4+messageLen])
		}
	}
}

func parseClientHelloExtensionOrder(hello []byte) ([]uint16, error) {
	offset := 2 + 32 // legacy_version + random
	if len(hello) < offset+1 {
		return nil, io.ErrUnexpectedEOF
	}
	offset += 1 + int(hello[offset]) // legacy_session_id
	if len(hello) < offset+2 {
		return nil, io.ErrUnexpectedEOF
	}
	cipherLen := int(binary.BigEndian.Uint16(hello[offset : offset+2]))
	offset += 2 + cipherLen
	if len(hello) < offset+1 {
		return nil, io.ErrUnexpectedEOF
	}
	offset += 1 + int(hello[offset]) // legacy_compression_methods
	if len(hello) < offset+2 {
		return nil, io.ErrUnexpectedEOF
	}
	extensionLen := int(binary.BigEndian.Uint16(hello[offset : offset+2]))
	offset += 2
	if len(hello) < offset+extensionLen {
		return nil, io.ErrUnexpectedEOF
	}
	end := offset + extensionLen
	order := make([]uint16, 0, 16)
	for offset < end {
		if end-offset < 4 {
			return nil, io.ErrUnexpectedEOF
		}
		id := binary.BigEndian.Uint16(hello[offset : offset+2])
		bodyLen := int(binary.BigEndian.Uint16(hello[offset+2 : offset+4]))
		offset += 4
		if end-offset < bodyLen {
			return nil, io.ErrUnexpectedEOF
		}
		order = append(order, id)
		offset += bodyLen
	}
	return order, nil
}

func sameUint16Multiset(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[uint16]int, len(a))
	for _, value := range a {
		counts[value]++
	}
	for _, value := range b {
		counts[value]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func cloneProfileForTest(profile *Profile) *Profile {
	clone := *profile
	clone.CipherSuites = append([]uint16(nil), profile.CipherSuites...)
	clone.Curves = append([]uint16(nil), profile.Curves...)
	clone.PointFormats = append([]uint16(nil), profile.PointFormats...)
	clone.SignatureAlgorithms = append([]uint16(nil), profile.SignatureAlgorithms...)
	clone.ALPNProtocols = append([]string(nil), profile.ALPNProtocols...)
	clone.SupportedVersions = append([]uint16(nil), profile.SupportedVersions...)
	clone.KeyShareGroups = append([]uint16(nil), profile.KeyShareGroups...)
	clone.PSKModes = append([]uint16(nil), profile.PSKModes...)
	clone.Extensions = append([]uint16(nil), profile.Extensions...)
	return &clone
}

func indexUint16(values []uint16, target uint16) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
