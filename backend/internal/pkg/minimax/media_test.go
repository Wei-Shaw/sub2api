package minimax

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMiniMaxMediaSpeechValidation(t *testing.T) {
	for _, model := range []string{SpeechHD, SpeechTurbo} {
		r, err := ParseRequest([]byte(fmt.Sprintf(`{"model":%q,"text":"你好 hello","voice_setting":{"voice_id":"system"}}`, model)), "speech")
		require.NoError(t, err)
		require.Equal(t, []string{"system"}, r.Voices())
		require.EqualValues(t, 10, r.Estimate("speech").Characters)
	}
	for _, body := range []string{
		`{"model":"speech-2.8-hd","Model":"other"}`,
		`{"model":"speech-2.8-hd","model":"speech-2.8-turbo"}`,
		`{"model":"speech-2.8-hd","text":"hi","voice_setting":{"voice_id":"a","voice_id":"b"}}`,
		`{"model":"speech-2.8-hd","text":"","voice_setting":{"voice_id":"a"}}`,
		`{"model":"speech-01-hd","text":"hi","voice_setting":{"voice_id":"a"}}`,
		`{"model":"speech-2.8-hd","text":"hi"}`, "null", `{} {}`,
	} {
		_, err := ParseRequest([]byte(body), "speech")
		require.Error(t, err, body)
	}
	_, err := ParseRequest([]byte(`{"model":"speech-2.8-hd","text":"`+strings.Repeat("中", 10000)+`","voice_setting":{"voice_id":"a"}}`), "speech")
	require.Error(t, err)
	r, err := ParseRequest([]byte(`{"model":"speech-2.8-hd","text":"hi","timbre_weights":[{"voice_id":"a"},{"voice_id":"custom"}]}`), "speech")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "custom"}, r.Voices())
}
func TestMiniMaxMediaH3Validation(t *testing.T) {
	for _, resolution := range []string{"768P", "2K"} {
		for _, duration := range []int{4, 15} {
			_, err := ParseRequest([]byte(fmt.Sprintf(`{"model":"MiniMax-H3","resolution":%q,"duration":%d,"ratio":"16:9","content":[{"type":"text","text":"a bird"}]}`, resolution, duration)), "video")
			require.NoError(t, err)
		}
	}
	for _, fields := range []string{`"duration":3`, `"duration":16`, `"duration":4.5`, `"duration":4,"callback_url":"https://example.com/callback"`, `"duration":4,"task_type":"context_ir"`, `"duration":4,"resolution":"720P"`} {
		_, err := ParseRequest([]byte(`{"model":"MiniMax-H3","resolution":"2K","ratio":"16:9","content":[{"type":"text","text":"a bird"}],`+fields+`}`), "video")
		require.Error(t, err)
	}
	_, err := ParseRequest([]byte(`{"model":"MiniMax-H3","resolution":"2K","duration":4,"content":[{"type":"text","text":"x"}]}`), "video")
	require.Error(t, err, "pure text needs ratio")
	_, err = ParseRequest([]byte(`{"model":"MiniMax-H3","resolution":"2K","duration":4,"content":[{"type":"text","text":"x"},{"type":"image_url","image_url":{"url":"https://example.com/1.png"},"role":"first_frame"},{"type":"video_url","video_url":{"url":"https://example.com/1.mp4"},"role":"reference_video"}]}`), "video")
	require.Error(t, err, "frame and reference modes must not mix")
	r, err := ParseRequest([]byte(`{"model":"MiniMax-H3","resolution":"2K","duration":4,"content":[{"type":"text","text":"x"},{"type":"image_url","image_url":{"url":"https://example.com/1.png"},"role":"reference_image"},{"type":"video_url","video_url":{"url":"https://example.com/1.mp4"},"role":"reference_video"}]}`), "video")
	require.NoError(t, err)
	require.Equal(t, Usage{OutputSeconds: 4, InputSeconds: 15, InputImages: 1}, r.Estimate("video"))
}
func TestMiniMaxMediaSpeechReceipt(t *testing.T) {
	u, final, err := SpeechUsage([]byte(`{"base_resp":{"status_code":0},"data":{"status":2},"extra_info":{"usage_characters":2000,"word_count":1000}}`))
	require.NoError(t, err)
	require.True(t, final)
	require.EqualValues(t, 2000, u.Characters)
	for _, body := range []string{`{"base_resp":{"status_code":1004},"data":null}`, `{"data":{"status":2},"extra_info":{"usage_characters":2}}`, `{"base_resp":{"status_code":0},"data":{"status":2},"extra_info":{"word_count":2000}}`, `{"base_resp":{"status_code":0},"data":{"status":2},"extra_info":{"usage_characters":2000,"usage_voice_count":-1}}`} {
		_, final, err := SpeechUsage([]byte(body))
		require.Error(t, err)
		require.False(t, final)
	}
	_, final, err = SpeechUsage([]byte(`{"base_resp":{"status_code":0},"data":{"status":1,"audio":"ab"}}`))
	require.NoError(t, err)
	require.False(t, final)
}
func TestMiniMaxMediaVideoReceipt(t *testing.T) {
	var task VideoTask
	require.NoError(t, Decode([]byte(`{"id":"upstream","model":"MiniMax-H3","resolution":"2K","usage":{"total_seconds":12,"output_seconds":6,"input_seconds":6,"input_image_count":9}}`), &task))
	u, err := task.Meter(H3, "2K")
	require.NoError(t, err)
	require.Equal(t, Usage{OutputSeconds: 6, InputSeconds: 6, InputImages: 9}, u)
	_, err = task.Meter(H3, "768P")
	require.Error(t, err)
	task.Usage.InputSeconds = nil
	_, err = task.Meter(H3, "2K")
	require.Error(t, err)
}
