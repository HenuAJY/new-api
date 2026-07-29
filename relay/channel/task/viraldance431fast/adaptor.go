package viraldance431fast

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type submitRequest struct {
	Model       string                    `json:"model"`
	Prompt      string                    `json:"prompt"`
	Content     []dto.SeedanceContentItem `json:"content,omitempty"`
	Duration    *dto.IntValue             `json:"duration,omitempty"`
	Seconds     *dto.IntValue             `json:"seconds,omitempty"`
	Ratio       string                    `json:"ratio,omitempty"`
	AspectRatio string                    `json:"aspect_ratio,omitempty"`
	Size        string                    `json:"size,omitempty"`
	Resolution  string                    `json:"resolution,omitempty"`
	Images      []string                  `json:"images,omitempty"`
	Videos      []string                  `json:"videos,omitempty"`
	Audios      []string                  `json:"audios,omitempty"`
}

type upstreamRequest struct {
	Model          string           `json:"model"`
	Prompt         string           `json:"prompt"`
	Duration       *dto.IntValue    `json:"duration,omitempty"`
	AspectRatio    string           `json:"aspect_ratio,omitempty"`
	Size           string           `json:"size,omitempty"`
	Resolution     string           `json:"resolution,omitempty"`
	ImageURLs      []string         `json:"image_urls,omitempty"`
	StartImageURL  string           `json:"start_image_url,omitempty"`
	EndImageURL    string           `json:"end_image_url,omitempty"`
	VideoReference []mediaReference `json:"video_reference,omitempty"`
	AudioReference []mediaReference `json:"audio_reference,omitempty"`
}

type mediaReference struct {
	URL string `json:"url"`
}

type taskResponse struct {
	Code        string       `json:"code,omitempty"`
	Message     string       `json:"message,omitempty"`
	ID          string       `json:"id,omitempty"`
	TaskID      string       `json:"task_id,omitempty"`
	Model       string       `json:"model,omitempty"`
	Status      string       `json:"status,omitempty"`
	Progress    int          `json:"progress,omitempty"`
	CreatedAt   int64        `json:"created_at,omitempty"`
	CompletedAt int64        `json:"completed_at,omitempty"`
	FailedAt    int64        `json:"failed_at,omitempty"`
	URL         string       `json:"url,omitempty"`
	VideoURL    string       `json:"video_url,omitempty"`
	Metadata    taskMetadata `json:"metadata,omitempty"`
	Error       *taskError   `json:"error,omitempty"`
}

type taskMetadata struct {
	URL string `json:"url,omitempty"`
}

type taskError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	if a.baseURL == "" {
		a.baseURL = defaultBaseURL
	}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var request submitRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	upstream := upstreamRequest{Prompt: strings.TrimSpace(request.Prompt), Size: strings.TrimSpace(request.Size)}

	upstream.Duration = request.Duration
	if upstream.Duration == nil {
		upstream.Duration = request.Seconds
	}
	if upstream.Duration != nil && (int(*upstream.Duration) < 4 || int(*upstream.Duration) > 15) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration must be between 4 and 15 seconds"), "invalid_request", http.StatusBadRequest)
	}
	if aspectRatio := firstNonEmpty(request.AspectRatio, request.Ratio); isSupportedAspectRatio(aspectRatio) {
		upstream.AspectRatio = aspectRatio
	}
	if strings.EqualFold(request.Resolution, "720p") {
		upstream.Resolution = "720p"
	}

	if len(request.Content) > 0 {
		for _, item := range request.Content {
			switch item.Type {
			case "text":
				if text := strings.TrimSpace(item.Text); upstream.Prompt == "" && text != "" {
					upstream.Prompt = text
				}
			case "image_url":
				if item.ImageURL == nil {
					continue
				}
				url := strings.TrimSpace(item.ImageURL.URL)
				if url == "" {
					continue
				}
				switch item.Role {
				case "first_frame":
					upstream.StartImageURL = url
				case "last_frame":
					upstream.EndImageURL = url
				default:
					upstream.ImageURLs = append(upstream.ImageURLs, url)
				}
			case "video_url":
				if item.VideoURL != nil {
					upstream.VideoReference = appendReference(upstream.VideoReference, item.VideoURL.URL)
				}
			case "audio_url":
				if item.AudioURL != nil {
					upstream.AudioReference = appendReference(upstream.AudioReference, item.AudioURL.URL)
				}
			}
		}
	} else {
		upstream.ImageURLs = cleanURLs(request.Images)
		for _, url := range request.Videos {
			upstream.VideoReference = appendReference(upstream.VideoReference, url)
		}
		for _, url := range request.Audios {
			upstream.AudioReference = appendReference(upstream.AudioReference, url)
		}
	}
	if upstream.Prompt == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if len([]rune(upstream.Prompt)) > 5000 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt must not exceed 5000 characters"), "invalid_request", http.StatusBadRequest)
	}

	if len(upstream.ImageURLs) > 4 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("at most 4 reference images are supported"), "invalid_request", http.StatusBadRequest)
	}
	if len(upstream.VideoReference) > 3 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("at most 3 reference videos are supported"), "invalid_request", http.StatusBadRequest)
	}
	if len(upstream.AudioReference) > 1 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("at most 1 reference audio is supported"), "invalid_request", http.StatusBadRequest)
	}
	for _, reference := range append(append([]mediaReference{}, upstream.VideoReference...), upstream.AudioReference...) {
		if strings.HasPrefix(strings.ToLower(reference.URL), "data:") {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video and audio references must use URLs"), "unsupported_reference_media", http.StatusBadRequest)
		}
	}

	info.Action = constant.TaskActionGenerate
	c.Set("viraldance431fast_task_request", upstream)
	seconds := ""
	if upstream.Duration != nil {
		seconds = strconv.Itoa(int(*upstream.Duration))
	}
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:  upstream.Prompt,
		Model:   request.Model,
		Images:  upstream.ImageURLs,
		Seconds: seconds,
		Size:    firstNonEmpty(upstream.Size, upstream.AspectRatio),
	})
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/videos", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, request *http.Request, _ *relaycommon.RelayInfo) error {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	value, ok := c.Get("viraldance431fast_task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	request, ok := value.(upstreamRequest)
	if !ok {
		return nil, fmt.Errorf("invalid task request type")
	}
	request.Model = firstNonEmpty(info.UpstreamModelName, info.OriginModelName, request.Model)
	if request.Model == "" {
		return nil, fmt.Errorf("upstream model is empty")
	}
	body, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = response.Body.Close()

	var result taskResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf(publicFailureMessage), "invalid_upstream_response", responseStatus(response))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || result.Error != nil {
		code := firstNonEmpty(result.Code, "upstream_request_failed")
		if result.Error != nil {
			code = firstNonEmpty(result.Error.Code, code)
		}
		return "", nil, service.TaskErrorWrapper(fmt.Errorf(publicFailureMessage), code, responseStatus(response))
	}
	taskID := firstNonEmpty(result.ID, result.TaskID)
	if taskID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf(publicFailureMessage), "invalid_upstream_response", http.StatusBadGateway)
	}

	if c.GetBool("seedance_api") {
		c.JSON(http.StatusOK, dto.SeedanceCreateResponse{ID: info.PublicTaskID})
	} else {
		video := dto.NewOpenAIVideo()
		video.ID = info.PublicTaskID
		video.TaskID = info.PublicTaskID
		video.Model = info.OriginModelName
		video.Status = toOpenAIVideoStatus(result.Status)
		video.Progress = result.Progress
		video.CreatedAt = time.Now().Unix()
		c.JSON(http.StatusOK, video)
	}
	return taskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	request, err := http.NewRequest(http.MethodGet, baseURL+"/v1/videos/"+taskID, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(request)
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var response taskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	result := &relaycommon.TaskInfo{
		Code:       0,
		Url:        firstNonEmpty(response.URL, response.VideoURL, response.Metadata.URL),
		Progress:   normalizeProgress(response.Progress),
		StartTime:  response.CreatedAt,
		FinishTime: firstPositive(response.CompletedAt, response.FailedAt),
	}
	switch strings.ToLower(response.Status) {
	case "submitted", "queued":
		result.Status = model.TaskStatusQueued
	case "processing", "in_progress", "running":
		result.Status = model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		if result.Url == "" {
			result.Status = model.TaskStatusInProgress
			result.Progress = taskcommon.ProgressInProgress
		} else {
			result.Status = model.TaskStatusSuccess
			result.Progress = taskcommon.ProgressComplete
		}
	case "failed":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		if response.Error != nil {
			result.Reason = response.Error.Message
		}
		if result.Reason == "" {
			result.Reason = firstNonEmpty(response.Message, "task failed")
		}
	default:
		return nil, fmt.Errorf("unknown task status: %s", response.Status)
	}
	if result.Status != model.TaskStatusSuccess && result.Status != model.TaskStatusFailure && result.Progress == taskcommon.ProgressComplete {
		result.Progress = "99%"
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var response taskResponse
	_ = common.Unmarshal(task.Data, &response)
	video := task.ToOpenAIVideo()
	video.TaskID = task.TaskID
	if task.Status == model.TaskStatusFailure {
		video.Metadata = nil
		video.Error = &dto.OpenAIVideoError{Message: publicFailureMessage}
		if response.Error != nil {
			video.Error.Code = response.Error.Code
		}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) ConvertToSeedanceVideo(task *model.Task) ([]byte, error) {
	var response taskResponse
	_ = common.Unmarshal(task.Data, &response)
	result := dto.SeedanceTaskResponse{
		ID:        task.TaskID,
		Model:     task.Properties.OriginModelName,
		Status:    toSeedanceStatus(task.Status),
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}
	if task.Status == model.TaskStatusSuccess {
		result.Content.VideoURL = task.GetResultURL()
	}
	if task.Status == model.TaskStatusFailure {
		result.Error = &dto.SeedanceTaskError{Message: publicFailureMessage}
		if response.Error != nil {
			result.Error.Code = response.Error.Code
		}
	}
	return common.Marshal(result)
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func isSupportedAspectRatio(ratio string) bool {
	switch ratio {
	case "16:9", "9:16", "1:1":
		return true
	default:
		return false
	}
}

func appendReference(references []mediaReference, rawURL string) []mediaReference {
	if url := strings.TrimSpace(rawURL); url != "" {
		return append(references, mediaReference{URL: url})
	}
	return references
}

func cleanURLs(urls []string) []string {
	result := make([]string, 0, len(urls))
	for _, rawURL := range urls {
		if url := strings.TrimSpace(rawURL); url != "" {
			result = append(result, url)
		}
	}
	return result
}

func toOpenAIVideoStatus(status string) string {
	switch strings.ToLower(status) {
	case "processing", "in_progress", "running":
		return dto.VideoStatusInProgress
	case "completed", "succeeded", "success":
		return dto.VideoStatusCompleted
	case "failed":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusQueued
	}
}

func toSeedanceStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusQueued, model.TaskStatusSubmitted, model.TaskStatusNotStart:
		return "queued"
	case model.TaskStatusInProgress:
		return "running"
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	default:
		return "queued"
	}
}

func normalizeProgress(progress int) string {
	if progress <= 0 {
		return ""
	}
	if progress > 100 {
		progress = 100
	}
	return fmt.Sprintf("%d%%", progress)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func responseStatus(response *http.Response) int {
	if response.StatusCode >= http.StatusBadRequest {
		return response.StatusCode
	}
	return http.StatusBadGateway
}
