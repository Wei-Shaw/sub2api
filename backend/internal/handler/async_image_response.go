package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	publicImageSubmitFailure = "Failed to submit image task. Please try again later."
	publicImageFailure       = "Image generation failed. Please try again later."
	publicImageAccountError  = "No available image account"
	publicVideoSubmitFailure = "Failed to submit video task. Please try again later."
	publicVideoFailure       = "Video generation failed. Please try again later."
)

// writeAsyncImageResult writes the normalized image result shared by the
// the protocol-neutral model facade.
func writeAsyncImageResult(c *gin.Context, task *service.AsyncMediaTask) {
	response := &fal.Response{Images: make([]fal.Image, 0)}
	if task != nil {
		for i, imageURL := range task.ResultURLs() {
			item := fal.Image{URL: imageURL}
			if i < len(task.ImageMetadata) {
				metadata := task.ImageMetadata[i]
				item.ContentType = metadata.ContentType
				item.FileName = metadata.FileName
				item.FileSize = metadata.FileSize
				item.Width = metadata.Width
				item.Height = metadata.Height
			}
			response.Images = append(response.Images, item)
		}
	}
	c.JSON(http.StatusOK, response)
}
