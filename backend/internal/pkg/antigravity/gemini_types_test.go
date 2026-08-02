package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiRequest_UnmarshalJSON(t *testing.T) {
	t.Run("unmarshals snake_case system_instruction", func(t *testing.T) {
		input := []byte(`{
			"system_instruction": {"parts": [{"text": "snake system"}]},
			"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
		}`)
		var req GeminiRequest
		err := json.Unmarshal(input, &req)
		require.NoError(t, err)

		require.NotNil(t, req.SystemInstruction)
		require.Len(t, req.SystemInstruction.Parts, 1)
		require.Equal(t, "snake system", req.SystemInstruction.Parts[0].Text)
		require.Len(t, req.Contents, 1)
	})

	t.Run("unmarshals proto wrapper _system_instruction", func(t *testing.T) {
		input := []byte(`{
			"_system_instruction": {"parts": [{"text": "proto system"}]},
			"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
		}`)
		var req GeminiRequest
		err := json.Unmarshal(input, &req)
		require.NoError(t, err)

		require.NotNil(t, req.SystemInstruction)
		require.Len(t, req.SystemInstruction.Parts, 1)
		require.Equal(t, "proto system", req.SystemInstruction.Parts[0].Text)
		require.Len(t, req.Contents, 1)
	})

	t.Run("moves system role in contents into SystemInstruction", func(t *testing.T) {
		input := []byte(`{
			"contents": [
				{"role": "system", "parts": [{"text": "system in contents"}]},
				{"role": "user", "parts": [{"text": "user message"}]}
			]
		}`)
		var req GeminiRequest
		err := json.Unmarshal(input, &req)
		require.NoError(t, err)

		require.NotNil(t, req.SystemInstruction)
		require.Len(t, req.SystemInstruction.Parts, 1)
		require.Equal(t, "system in contents", req.SystemInstruction.Parts[0].Text)

		require.Len(t, req.Contents, 1)
		require.Equal(t, "user", req.Contents[0].Role)
	})
}
