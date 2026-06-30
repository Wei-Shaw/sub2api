package tlsfingerprint

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseCapturedClientHelloFromUTLSDialer(t *testing.T) {
	profile := &Profile{
		Name:                "capture test",
		EnableGREASE:        true,
		CipherSuites:        []uint16{0x1301, 0x1302, 0x1303},
		Curves:              []uint16{0x001d, 0x0017},
		PointFormats:        []uint16{0},
		SignatureAlgorithms: []uint16{0x0403, 0x0804, 0x0401},
		ALPNProtocols:       []string{"h2", "http/1.1"},
		SupportedVersions:   []uint16{0x0304, 0x0303},
		KeyShareGroups:      []uint16{0x001d},
		PSKModes:            []uint16{1},
		Extensions:          []uint16{0x0a0a, 0, 10, 11, 13, 16, 43, 45, 51},
	}
	recordCh := make(chan []byte, 1)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		reader := bufio.NewReader(conn)
		header, err := reader.Peek(5)
		if err != nil {
			return
		}
		recordLen := int(header[3])<<8 | int(header[4])
		record, err := reader.Peek(5 + recordLen)
		if err != nil {
			return
		}
		copied := make([]byte, len(record))
		copy(copied, record)
		recordCh <- copied
	}()

	dialer := NewDialer(profile, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialer.DialTLSContext(ctx, "tcp", ln.Addr().String())
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)

	record := <-recordCh
	captured, err := ParseCapturedClientHello(record)
	require.NoError(t, err)
	require.Equal(t, []uint16{0x1301, 0x1302, 0x1303}, captured.CipherSuites)
	require.Equal(t, uint16(0x0a0a), captured.Extensions[0])
	require.Subset(t, captured.Extensions, []uint16{10, 11, 13, 16, 43, 45, 51})
	require.Equal(t, []uint16{0x001d, 0x0017}, captured.Curves)
	require.Equal(t, []uint16{0}, captured.PointFormats)
	require.Equal(t, []uint16{0x0403, 0x0804, 0x0401}, captured.SignatureAlgorithms)
	require.Equal(t, []string{"h2", "http/1.1"}, captured.ALPNProtocols)
	require.Equal(t, []uint16{0x0304, 0x0303}, captured.SupportedVersions)
	require.Equal(t, []uint16{0x001d}, captured.KeyShareGroups)
	require.Equal(t, []uint16{1}, captured.PSKModes)
	require.True(t, captured.EnableGREASE)
	require.NotEmpty(t, captured.JA3Raw)
	require.Len(t, captured.JA3Hash, 32)
}
