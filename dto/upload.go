package dto

type UploadPresignRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
}

type UploadPresignResponse struct {
	URL       string            `json:"url"`
	PublicURL string            `json:"public_url"`
	Method    string            `json:"method"`
	ObjectKey string            `json:"object_key"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt int64             `json:"expires_at"`
}
