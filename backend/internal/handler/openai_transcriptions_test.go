package handler

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenAITranscriptionRequestRequiresModelAndAudioFile(t *testing.T) {
	var valid bytes.Buffer
	writer := multipart.NewWriter(&valid)
	require.NoError(t, writer.WriteField("model", "fun-asr-nano"))
	file, err := writer.CreateFormFile("file", "sample.wav")
	require.NoError(t, err)
	_, err = file.Write([]byte("RIFFaudio"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	parsed, err := parseOpenAITranscriptionRequest(valid.Bytes(), writer.FormDataContentType())
	require.NoError(t, err)
	require.Equal(t, "fun-asr-nano", parsed.Model)

	var missingFile bytes.Buffer
	missingFileWriter := multipart.NewWriter(&missingFile)
	require.NoError(t, missingFileWriter.WriteField("model", "fun-asr-nano"))
	require.NoError(t, missingFileWriter.Close())
	_, err = parseOpenAITranscriptionRequest(missingFile.Bytes(), missingFileWriter.FormDataContentType())
	require.EqualError(t, err, "file is required")

	var missingModel bytes.Buffer
	missingModelWriter := multipart.NewWriter(&missingModel)
	file, err = missingModelWriter.CreateFormFile("file", "sample.wav")
	require.NoError(t, err)
	_, err = file.Write([]byte("RIFFaudio"))
	require.NoError(t, err)
	require.NoError(t, missingModelWriter.Close())
	_, err = parseOpenAITranscriptionRequest(missingModel.Bytes(), missingModelWriter.FormDataContentType())
	require.EqualError(t, err, "model is required")
}
