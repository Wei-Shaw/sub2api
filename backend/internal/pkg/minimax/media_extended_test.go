package minimax

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMiniMaxExtendedAdmissionAndMeters(t *testing.T) {
	for _, m := range []string{"speech-2.6-hd", "speech-02-turbo"} {
		_, e := ParseRequest([]byte(fmt.Sprintf(`{"model":%q,"text":"hi","voice_setting":{"voice_id":"voice"}}`, m)), "speech")
		require.NoError(t, e)
	}
	for _, tc := range []struct {
		m, r string
		d    int
		ok   bool
	}{{H3Max, "480P", 5, true}, {H3Max, "768P", 4, false}, {H3Lite, "480P", 4, true}, {H3Highspeed, "2K", 4, false}} {
		_, e := ParseRequest([]byte(fmt.Sprintf(`{"model":%q,"resolution":%q,"duration":%d,"ratio":"16:9","content":[{"type":"text","text":"bird"}]}`, tc.m, tc.r, tc.d)), "video")
		require.Equal(t, tc.ok, e == nil)
	}
	r, e := ParseRequest([]byte(`{"file_id":123456789012345678,"voice_id":"My_voice01"}`), "voice_clone")
	require.NoError(t, e)
	require.Equal(t, "123456789012345678", string(r.FileID))
	require.Equal(t, "voice_clone", r.Model)
	_, e = ParseRequest([]byte(`{"file_id":1,"voice_id":"bad_id_"}`), "voice_clone")
	require.Error(t, e)
	_, e = ParseRequest([]byte(`{"file_id":1,"voice_id":"ValidVoice","text":"hi"}`), "voice_clone")
	require.Error(t, e)
	r, e = ParseRequest([]byte(`{"prompt":"warm voice","preview_text":"你好 world"}`), "voice_design")
	require.NoError(t, e)
	require.EqualValues(t, 8, r.Estimate("voice_design").Characters)
	for _, count := range []string{`3`, `"3"`} {
		u, e := ImageUsage([]byte(`{"base_resp":{"status_code":0},"metadata":{"success_count":` + count + `}}`))
		require.NoError(t, e)
		require.Equal(t, 3, u.OutputImages)
	}
	for _, count := range []string{`-1`, `10`, `1.5`, `null`} {
		_, e = ImageUsage([]byte(`{"base_resp":{"status_code":0},"metadata":{"success_count":` + count + `}}`))
		require.Error(t, e)
	}
	_, e = VoicePreviewUsage([]byte(`{"base_resp":{"status_code":0},"extra_info":{"usage_characters":10}}`), true)
	require.NoError(t, e)
	_, e = VoicePreviewUsage([]byte(`{"base_resp":{"status_code":0},"demo_audio":"url"}`), true)
	require.Error(t, e)
	_, e = VoicePreviewUsage([]byte(`{"base_resp":{"status_code":0},"extra_info":{"usage_voice_count":1}}`), false)
	require.Error(t, e)
}

func TestMiniMaxVoiceDesignMeterSource(t *testing.T) {
	for _, tc := range []struct {
		name, receipt, source string
		characters            int64
		valid                 bool
	}{
		{"provider meter", `{"base_resp":{"status_code":0},"extra_info":{"usage_characters":99}}`, "upstream_usage", 99, true},
		{"input price", `{"base_resp":{"status_code":0},"trial_audio":"encoded-audio"}`, "request_characters", 3, true},
		{"partial meter", `{"base_resp":{"status_code":0},"trial_audio":"encoded-audio","extra_info":{}}`, "", 0, false},
		{"null meter", `{"base_resp":{"status_code":0},"trial_audio":"encoded-audio","extra_info":null}`, "", 0, false},
		{"missing audio", `{"base_resp":{"status_code":0}}`, "", 0, false},
		{"failed design", `{"base_resp":{"status_code":1000},"trial_audio":"encoded-audio"}`, "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, source, err := VoiceDesignUsage([]byte(tc.receipt), "你好a")
			require.Equal(t, tc.valid, err == nil)
			require.Equal(t, tc.source, source)
			require.Equal(t, tc.characters, u.Characters)
		})
	}
}
