package dto

type SeedanceContentItem struct {
	Type      string             `json:"type"`
	Text      string             `json:"text,omitempty"`
	ImageURL  *SeedanceMediaURL  `json:"image_url,omitempty"`
	VideoURL  *SeedanceMediaURL  `json:"video_url,omitempty"`
	AudioURL  *SeedanceMediaURL  `json:"audio_url,omitempty"`
	DraftTask *SeedanceDraftTask `json:"draft_task,omitempty"`
	Role      string             `json:"role,omitempty"`
}

type SeedanceDraftTask struct {
	ID string `json:"id"`
}

type SeedanceTool struct {
	Type string `json:"type"`
}

type SeedanceTaskRequest struct {
	Model                 string                `json:"model"`
	Content               []SeedanceContentItem `json:"content"`
	CallbackURL           string                `json:"callback_url,omitempty"`
	ReturnLastFrame       *bool                 `json:"return_last_frame,omitempty"`
	ServiceTier           string                `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *int                  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *bool                 `json:"generate_audio,omitempty"`
	Draft                 *bool                 `json:"draft,omitempty"`
	Tools                 []SeedanceTool        `json:"tools,omitempty"`
	SafetyIdentifier      string                `json:"safety_identifier,omitempty"`
	Priority              *int                  `json:"priority,omitempty"`
	Resolution            string                `json:"resolution,omitempty"`
	Ratio                 string                `json:"ratio,omitempty"`
	Duration              *int                  `json:"duration,omitempty"`
	Frames                *int                  `json:"frames,omitempty"`
	Seed                  *int                  `json:"seed,omitempty"`
	CameraFixed           *bool                 `json:"camera_fixed,omitempty"`
	Watermark             *bool                 `json:"watermark,omitempty"`
}

type SeedanceCreateResponse struct {
	ID string `json:"id"`
}

type SeedanceTaskContent struct {
	VideoURL string `json:"video_url,omitempty"`
}

type SeedanceTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SeedanceTaskResponse struct {
	ID         string              `json:"id"`
	Model      string              `json:"model"`
	Status     string              `json:"status"`
	Error      *SeedanceTaskError  `json:"error"`
	CreatedAt  int64               `json:"created_at"`
	UpdatedAt  int64               `json:"updated_at"`
	Content    SeedanceTaskContent `json:"content"`
	Resolution string              `json:"resolution,omitempty"`
	Ratio      string              `json:"ratio,omitempty"`
	Duration   *int                `json:"duration,omitempty"`
}
