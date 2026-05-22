package domain

type ChatMessageAttachment struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
	MimeType string `json:"mime_type"`
	Name     string `json:"name,omitempty"`
	Size     int64  `json:"size,omitempty"`
}
