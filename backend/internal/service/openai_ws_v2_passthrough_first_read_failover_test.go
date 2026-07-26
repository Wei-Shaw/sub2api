//go:build unit

package service

import (
	"errors"
	"io"
	"testing"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSPassthroughRelayClientClose_FirstReadFailureWithoutDownstreamWrite(t *testing.T) {
	exit := openaiwsv2.RelayExit{
		Stage:           "read_upstream",
		Err:             io.EOF,
		Graceful:        false,
		WroteDownstream: false,
	}

	code, reason, shouldClose := openAIWSPassthroughRelayClientClose(exit, 0)

	require.Zero(t, code, "first-turn read_upstream failure without downstream write should return 0 to trigger failover")
	require.Empty(t, reason)
	require.False(t, shouldClose)
}

func TestOpenAIWSPassthroughRelayClientClose_FirstReadFailureAfterDownstreamWrite(t *testing.T) {
	exit := openaiwsv2.RelayExit{
		Stage:           "read_upstream",
		Err:             io.EOF,
		Graceful:        false,
		WroteDownstream: true,
	}

	code, reason, shouldClose := openAIWSPassthroughRelayClientClose(exit, 0)

	require.Equal(t, coderws.StatusInternalError, code)
	require.Equal(t, "upstream websocket proxy failed", reason)
	require.True(t, shouldClose)
}

func TestOpenAIWSPassthroughRelayClientClose_LaterTurnReadFailureWithoutDownstreamWrite(t *testing.T) {
	exit := openaiwsv2.RelayExit{
		Stage:           "read_upstream",
		Err:             io.EOF,
		Graceful:        false,
		WroteDownstream: false,
	}

	code, reason, shouldClose := openAIWSPassthroughRelayClientClose(exit, 1)

	require.Equal(t, coderws.StatusInternalError, code)
	require.Equal(t, "upstream websocket proxy failed", reason)
	require.True(t, shouldClose, "later turns cannot be replayed even without downstream write")
}

func TestOpenAIWSPassthroughRelayClientClose_GracefulReadUpstreamDoesNotTrigger1011(t *testing.T) {
	exit := openaiwsv2.RelayExit{
		Stage:           "read_upstream",
		Err:             io.EOF,
		Graceful:        true,
		WroteDownstream: false,
	}

	code, reason, shouldClose := openAIWSPassthroughRelayClientClose(exit, 0)

	require.Zero(t, code, "graceful read_upstream should not return 1011")
	require.Empty(t, reason)
	require.False(t, shouldClose)
}

func TestOpenAIWSPassthroughRelayClientClose_WriteUpstreamFailurePreservesFailover(t *testing.T) {
	exit := openaiwsv2.RelayExit{
		Stage:           "write_upstream",
		Err:             errors.New("connection reset by peer"),
		Graceful:        false,
		WroteDownstream: false,
	}

	code, reason, shouldClose := openAIWSPassthroughRelayClientClose(exit, 0)

	require.Zero(t, code, "write_upstream stage is not read_upstream so should not return 1011")
	require.Empty(t, reason)
	require.False(t, shouldClose)
}
