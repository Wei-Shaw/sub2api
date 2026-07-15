package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedOpenAIForwardImageIntentHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		channelMapped bool
		imageIntent   bool
		wantHint      bool
REDACTED{
		{name: "seed true", imageIntent: true, wantHint: trueREDACTED,
		{name: "seed false", imageIntent: false, wantHint: trueREDACTED,
		{name: "mapped body stays unknown", channelMapped: true, imageIntent: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &gin.Context{REDACTED
			service.SetOpenAIClientTransport(c, service.OpenAIClientTransportHTTP)

			seedOpenAIForwardImageIntentHint(c, tt.channelMapped, tt.imageIntent)

			var hintValues []bool
			for _, value := range c.Keys {
				if hint, ok := value.(bool); ok {
					hintValues = append(hintValues, hint)
			REDACTED
		REDACTED
			if !tt.wantHint {
				require.Empty(t, hintValues)
				return
		REDACTED
			require.Equal(t, []bool{tt.imageIntentREDACTED, hintValues)
	REDACTED)
REDACTED
REDACTED
