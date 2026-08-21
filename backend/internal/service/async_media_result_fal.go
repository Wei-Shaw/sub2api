package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
)

// extractFalImageResult adapts the FAL response into the common task result.
func extractFalImageResult(resp *fal.Response) ([]string, []string) {
	if resp == nil {
		return nil, nil
	}
	urls := make([]string, 0, len(resp.Images))
	sizes := make([]string, 0, len(resp.Images))
	for _, img := range resp.Images {
		if u := strings.TrimSpace(img.URL); u != "" {
			urls = append(urls, u)
			size := ""
			if img.Width > 0 && img.Height > 0 {
				size = fmt.Sprintf("%dx%d", img.Width, img.Height)
			}
			sizes = append(sizes, size)
		}
	}
	return urls, sizes
}

func extractFalImageMetadata(resp *fal.Response) []ImageOutputMetadata {
	if resp == nil {
		return nil
	}
	metadata := make([]ImageOutputMetadata, 0, len(resp.Images))
	for _, img := range resp.Images {
		if u := strings.TrimSpace(img.URL); u != "" {
			fileName := strings.TrimSpace(img.FileName)
			if fileName == "" {
				fileName = imageFileNameFromURL(u)
			}
			metadata = append(metadata, ImageOutputMetadata{
				URL: u, ContentType: strings.TrimSpace(img.ContentType), FileName: fileName,
				FileSize: img.FileSize, Width: img.Width, Height: img.Height,
			})
		}
	}
	return metadata
}
