package service

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/leonardo"
)

// extractLeonardoImageResult adapts Leonardo output.media into the common task result.
func extractLeonardoImageResult(task *leonardo.Task) ([]string, []string, []ImageOutputMetadata) {
	if task == nil {
		return nil, nil, nil
	}
	urls := make([]string, 0, len(task.Output.Media))
	sizes := make([]string, 0, len(task.Output.Media))
	metadata := make([]ImageOutputMetadata, 0, len(task.Output.Media))
	for _, media := range task.Output.Media {
		if imageURL := strings.TrimSpace(media.URL); imageURL != "" {
			urls = append(urls, imageURL)
			size := ""
			if media.Width > 0 && media.Height > 0 {
				size = fmt.Sprintf("%dx%d", media.Width, media.Height)
			}
			sizes = append(sizes, size)
			contentType := strings.TrimSpace(media.Type)
			if contentType == "" {
				contentType = strings.TrimSpace(media.MediaType)
			}
			if contentType == "" {
				contentType = strings.TrimSpace(media.MIMEType)
			}
			fileName := strings.TrimSpace(media.FileName)
			if fileName == "" {
				fileName = imageFileNameFromURL(imageURL)
			}
			metadata = append(metadata, ImageOutputMetadata{
				URL: imageURL, ContentType: contentType, FileName: fileName,
				FileSize: media.FileSize, Width: media.Width, Height: media.Height,
			})
		}
	}
	return urls, sizes, metadata
}

func imageFileNameFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return ""
	}
	return name
}
