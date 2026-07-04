package apimart

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
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

type inputReference struct {
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
}

type submitRequest struct {
	Prompt         string          `json:"prompt"`
	Model          string          `json:"model,omitempty"`
	Images         []string        `json:"images,omitempty"`
	Image          string          `json:"image,omitempty"`
	InputReference *inputReference `json:"input_reference,omitempty"`
	Seconds        string          `json:"seconds,omitempty"`
	Size           string          `json:"size,omitempty"`
}

type requestPayload struct {
	Model     string   `json:"model"`
	Prompt    string   `json:"prompt"`
	Size      string   `json:"size,omitempty"`
	Duration  int      `json:"duration"`
	ImageURLs []string `json:"image_urls,omitempty"`
}

type submitItem struct {
	TaskID string `json:"task_id,omitempty"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status,omitempty"`
}

type submitResponse struct {
	Code  int          `json:"code,omitempty"`
	Data  []submitItem `json:"data,omitempty"`
	Error *apiError    `json:"error,omitempty"`
}

type taskResponse struct {
	Code  int       `json:"code,omitempty"`
	Data  *taskData `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
}

type taskData struct {
	ID        string      `json:"id,omitempty"`
	TaskID    string      `json:"task_id,omitempty"`
	Status    string      `json:"status,omitempty"`
	Progress  int         `json:"progress,omitempty"`
	Result    *taskResult `json:"result,omitempty"`
	Created   int64       `json:"created,omitempty"`
	Completed int64       `json:"completed,omitempty"`
	Error     *apiError   `json:"error,omitempty"`
}

type taskResult struct {
	Videos []mediaResult `json:"videos,omitempty"`
}

type mediaResult struct {
	URL       any   `json:"url,omitempty"`
	ExpiresAt int64 `json:"expires_at,omitempty"`
}

type apiError struct {
	Code    any    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req submitRequest
	if strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_multipart", http.StatusBadRequest)
		}
		c.Request.MultipartForm = form
		if values := form.Value["model"]; len(values) > 0 {
			req.Model = values[0]
		}
		if values := form.Value["prompt"]; len(values) > 0 {
			req.Prompt = values[0]
		}
		if values := form.Value["size"]; len(values) > 0 {
			req.Size = values[0]
		}
		if values := form.Value["image"]; len(values) > 0 {
			req.Image = values[0]
		}
		if values := form.Value["images"]; len(values) > 0 {
			req.Images = values
		}
	} else if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_json", http.StatusBadRequest)
	}

	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}
	if req.Model != DownstreamModel {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported model: %s", req.Model), "unsupported_model", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if req.InputReference != nil && strings.TrimSpace(req.InputReference.FileID) != "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("input_reference.file_id is not supported by this upstream"), "unsupported_reference_file", http.StatusBadRequest)
	}

	imageURLs := normalizeImageURLs(req)
	for _, imageURL := range imageURLs {
		if strings.HasPrefix(strings.ToLower(imageURL), "data:") {
			return service.TaskErrorWrapperLocal(fmt.Errorf("APIMart image_urls only supports public URLs"), "unsupported_reference_image", http.StatusBadRequest)
		}
	}

	payload := requestPayload{
		Model:     upstreamModel,
		Prompt:    req.Prompt,
		Size:      normalizeSize(req.Size),
		Duration:  1,
		ImageURLs: imageURLs,
	}
	info.Action = constant.TaskActionGenerate
	c.Set("apimart_task_request", payload)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:  req.Prompt,
		Model:   req.Model,
		Images:  imageURLs,
		Seconds: "1",
		Size:    payload.Size,
	})
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/videos/generations", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, _ *relaycommon.RelayInfo) (io.Reader, error) {
	payload, ok := c.Get("apimart_task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := payload.(requestPayload)
	if !ok {
		return nil, fmt.Errorf("invalid task request type")
	}
	body, err := common.Marshal(req)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var response submitResponse
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if response.Error != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", response.Error.Message), "upstream_error", http.StatusBadGateway)
	}
	if len(response.Data) == 0 {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	upstreamTaskID := firstNonEmpty(response.Data[0].TaskID, response.Data[0].ID)
	if upstreamTaskID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.CreatedAt = time.Now().Unix()
	video.Model = info.OriginModelName
	video.Seconds = "6"
	video.Status = mapStatusToVideoStatus(response.Data[0].Status)
	video.Progress = mapStatusToProgress(response.Data[0].Status)
	c.JSON(http.StatusOK, video)
	return upstreamTaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tasks/%s?language=zh", baseUrl, taskID), nil)
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

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var response taskResponse
	if err := common.Unmarshal(respBody, &response); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	if response.Error != nil {
		return nil, fmt.Errorf("%s", response.Error.Message)
	}
	if response.Data == nil {
		return nil, fmt.Errorf("task data is empty")
	}

	taskResult := &relaycommon.TaskInfo{
		Code:       0,
		Url:        extractVideoURL(response.Data.Result),
		StartTime:  response.Data.Created,
		FinishTime: response.Data.Completed,
	}
	taskResult.Progress = normalizeProgress(response.Data.Progress)
	switch strings.ToLower(response.Data.Status) {
	case "pending", "submitted":
		taskResult.Status = model.TaskStatusQueued
		if taskResult.Progress == "" {
			taskResult.Progress = taskcommon.ProgressQueued
		}
	case "processing", "running", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
		if taskResult.Progress == "" {
			taskResult.Progress = taskcommon.ProgressInProgress
		}
	case "completed", "succeeded", "success":
		taskResult.Status = model.TaskStatusSuccess
		if taskResult.Progress == "" {
			taskResult.Progress = taskcommon.ProgressComplete
		}
	case "failed", "cancelled", "canceled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = taskcommon.ProgressComplete
		if response.Data.Error != nil && response.Data.Error.Message != "" {
			taskResult.Reason = response.Data.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		if taskResult.Url != "" {
			taskResult.Status = model.TaskStatusSuccess
			taskResult.Progress = taskcommon.ProgressComplete
		} else {
			taskResult.Status = model.TaskStatusInProgress
			if taskResult.Progress == "" {
				taskResult.Progress = taskcommon.ProgressInProgress
			}
		}
	}
	return taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var response taskResponse
	_ = common.Unmarshal(originTask.Data, &response)
	video := dto.NewOpenAIVideo()
	video.ID = originTask.TaskID
	video.TaskID = originTask.TaskID
	video.Status = originTask.Status.ToVideoStatus()
	video.SetProgressStr(originTask.Progress)
	video.CreatedAt = originTask.CreatedAt
	video.CompletedAt = originTask.UpdatedAt
	video.Model = originTask.Properties.OriginModelName
	video.Seconds = "6"
	if response.Data != nil {
		if url := extractVideoURL(response.Data.Result); url != "" {
			video.SetMetadata("url", url)
		}
		if response.Data.Error != nil {
			video.Error = &dto.OpenAIVideoError{Code: fmt.Sprint(response.Data.Error.Code), Message: response.Data.Error.Message}
		}
	}
	return common.Marshal(video)
}

func normalizeImageURLs(req submitRequest) []string {
	images := make([]string, 0, len(req.Images)+1)
	if req.InputReference != nil {
		if image := strings.TrimSpace(req.InputReference.ImageURL); image != "" {
			images = append(images, image)
		}
	}
	for _, image := range req.Images {
		if image = strings.TrimSpace(image); image != "" {
			images = append(images, image)
		}
	}
	if image := strings.TrimSpace(req.Image); image != "" {
		images = append(images, image)
	}
	return images
}

func normalizeSize(size string) string {
	switch strings.TrimSpace(size) {
	case "720x1280", "1024x1792":
		return "9:16"
	case "1280x720", "1792x1024":
		return "16:9"
	default:
		return "16:9"
	}
}

func mapStatusToVideoStatus(status string) string {
	switch strings.ToLower(status) {
	case "completed", "succeeded", "success":
		return dto.VideoStatusCompleted
	case "processing", "running", "in_progress":
		return dto.VideoStatusInProgress
	case "failed", "cancelled", "canceled":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusQueued
	}
}

func mapStatusToProgress(status string) int {
	switch mapStatusToVideoStatus(status) {
	case dto.VideoStatusCompleted, dto.VideoStatusFailed:
		return 100
	case dto.VideoStatusInProgress:
		return 30
	default:
		return 0
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

func extractVideoURL(result *taskResult) string {
	if result == nil || len(result.Videos) == 0 {
		return ""
	}
	switch url := result.Videos[0].URL.(type) {
	case string:
		return strings.TrimSpace(url)
	case []any:
		if len(url) > 0 {
			if value, ok := url[0].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	case []string:
		if len(url) > 0 {
			return strings.TrimSpace(url[0])
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
