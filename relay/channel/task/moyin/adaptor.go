package moyin

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
	Model       string           `json:"model"`
	Prompt      string           `json:"prompt"`
	Seconds     *dto.StringValue `json:"seconds,omitempty"`
	Duration    *dto.StringValue `json:"duration,omitempty"`
	AspectRatio string           `json:"aspect_ratio,omitempty"`
	Resolution  string           `json:"resolution,omitempty"`
	Size        string           `json:"size,omitempty"`
	Images      []string         `json:"images,omitempty"`
	Videos      []string         `json:"videos,omitempty"`
	Audios      []string         `json:"audios,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	Parameters  map[string]any   `json:"parameters,omitempty"`
}

type taskResponse struct {
	Code        any        `json:"code,omitempty"`
	Message     string     `json:"message,omitempty"`
	ID          string     `json:"id,omitempty"`
	TaskID      string     `json:"task_id,omitempty"`
	Model       string     `json:"model,omitempty"`
	Status      string     `json:"status,omitempty"`
	Progress    int        `json:"progress,omitempty"`
	CreatedAt   int64      `json:"created_at,omitempty"`
	CompletedAt int64      `json:"completed_at,omitempty"`
	FailedAt    int64      `json:"failed_at,omitempty"`
	Size        string     `json:"size,omitempty"`
	Seconds     string     `json:"seconds,omitempty"`
	VideoURL    string     `json:"video_url,omitempty"`
	Result      taskResult `json:"result,omitempty"`
	Error       *taskError `json:"error,omitempty"`
}

type taskResult struct {
	VideoURL string `json:"video_url,omitempty"`
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
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var request submitRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model is required"), "missing_model", http.StatusBadRequest)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	for _, mediaURL := range append(append([]string{}, request.Videos...), request.Audios...) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaURL)), "data:") {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video and audio references must use URLs"), "unsupported_reference_media", http.StatusBadRequest)
		}
	}
	if request.Seconds == nil {
		request.Seconds = request.Duration
	}
	request.Duration = nil
	info.Action = constant.TaskActionGenerate
	c.Set("moyin_task_request", request)

	metadata := map[string]any{
		"videos": request.Videos, "audios": request.Audios,
		"aspect_ratio": request.AspectRatio, "resolution": request.Resolution,
	}
	seconds := ""
	if request.Seconds != nil {
		seconds = string(*request.Seconds)
	}
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt: request.Prompt, Model: request.Model, Images: request.Images,
		Seconds: seconds, Size: request.Size, Metadata: metadata,
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
	value, ok := c.Get("moyin_task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	request, ok := value.(submitRequest)
	if !ok {
		return nil, fmt.Errorf("invalid task request type")
	}
	request.Model = info.UpstreamModelName
	if request.Model == "" {
		request.Model = info.OriginModelName
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
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", body), "unmarshal_response_body_failed", http.StatusBadGateway)
	}
	if result.Error != nil && result.Error.Message != "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", result.Error.Message), result.Error.Code, http.StatusBadGateway)
	}
	taskID := firstNonEmpty(result.ID, result.TaskID)
	if taskID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task id is empty"), "invalid_response", http.StatusBadGateway)
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
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/videos/"+taskID, nil)
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
	result := &relaycommon.TaskInfo{Code: 0}
	result.Url = firstNonEmpty(response.VideoURL, response.Result.VideoURL)
	result.Progress = normalizeProgress(response.Progress)
	result.StartTime = response.CreatedAt
	result.FinishTime = firstPositive(response.CompletedAt, response.FailedAt)

	switch strings.ToLower(response.Status) {
	case "queued", "submitted":
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
	return result, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var response taskResponse
	_ = common.Unmarshal(task.Data, &response)
	video := task.ToOpenAIVideo()
	video.TaskID = task.TaskID
	if response.Seconds != "" {
		video.Seconds = response.Seconds
	}
	if response.Size != "" {
		video.Size = response.Size
	}
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
		ID: task.TaskID, Model: task.Properties.OriginModelName,
		Status: toSeedanceStatus(task.Status), CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
		Ratio:     response.Size,
	}
	if task.Status == model.TaskStatusSuccess {
		result.Content.VideoURL = task.GetResultURL()
	}
	if seconds, err := strconv.Atoi(response.Seconds); err == nil && response.Seconds != "" {
		result.Duration = &seconds
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
