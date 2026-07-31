package aicost

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
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
	Model   string   `json:"model"`
	Prompt  string   `json:"prompt"`
	Images  []string `json:"images"`
	Seconds string   `json:"seconds"`
	Size    string   `json:"size"`
}

type responsePayload struct {
	ID       string            `json:"id,omitempty"`
	TaskID   string            `json:"task_id,omitempty"`
	Status   string            `json:"status,omitempty"`
	VideoURL string            `json:"video_url,omitempty"`
	Error    *openAIVideoError `json:"error,omitempty"`
}

type responseEnvelope struct {
	responsePayload
	Data *responsePayload `json:"data,omitempty"`
}

type openAIVideoError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
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

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	var req submitRequest
	var uploadedImages []string
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
		if values := form.Value["seconds"]; len(values) > 0 {
			req.Seconds = values[0]
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
		if fileHeaders := form.File["input_reference"]; len(fileHeaders) > 0 {
			file, err := fileHeaders[0].Open()
			if err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_reference_file", http.StatusBadRequest)
			}
			fileBytes, err := io.ReadAll(file)
			closeErr := file.Close()
			if err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_reference_file", http.StatusBadRequest)
			}
			if closeErr != nil {
				return service.TaskErrorWrapperLocal(closeErr, "invalid_reference_file", http.StatusBadRequest)
			}
			contentType := strings.TrimSpace(fileHeaders[0].Header.Get("Content-Type"))
			if contentType == "" || contentType == "application/octet-stream" {
				contentType = http.DetectContentType(fileBytes)
			}
			uploadedImages = append(uploadedImages, fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(fileBytes)))
		}
	} else {
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_json", http.StatusBadRequest)
		}
	}

	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}
	if req.Model != "grok-imagine-video-1.5-preview" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported model: %s", req.Model), "unsupported_model", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if req.InputReference != nil && strings.TrimSpace(req.InputReference.FileID) != "" && strings.TrimSpace(req.InputReference.ImageURL) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("input_reference.file_id is not supported by this upstream"), "unsupported_reference_file", http.StatusBadRequest)
	}

	images := normalizeImages(req)
	images = append(images, uploadedImages...)
	if len(images) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("input_reference.image_url or images is required"), "missing_reference_image", http.StatusBadRequest)
	}

	seconds := normalizeSeconds(req.Seconds)
	if !isSupportedSeconds(seconds) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("seconds must be an integer between 1 and 15"), "invalid_seconds", http.StatusBadRequest)
	}

	size := normalizeSize(req.Size)
	info.Action = constant.TaskActionGenerate
	payload := requestPayload{
		Model:   req.Model,
		Prompt:  req.Prompt,
		Images:  images,
		Seconds: seconds,
		Size:    size,
	}
	c.Set("aicost_task_request", payload)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:  req.Prompt,
		Model:   req.Model,
		Images:  images,
		Seconds: seconds,
		Size:    size,
	})
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	payload, ok := c.Get("aicost_task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := payload.(requestPayload)
	if !ok {
		return nil, fmt.Errorf("invalid task request type")
	}
	if info.UpstreamModelName != "" {
		req.Model = info.UpstreamModelName
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

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	response, err := parseResponsePayload(responseBody)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamTaskID := firstNonEmpty(response.TaskID, response.ID)
	if upstreamTaskID == "" {
		if response.VideoURL == "" {
			taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
			return
		}
		upstreamTaskID = info.PublicTaskID
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.CreatedAt = time.Now().Unix()
	video.Model = info.OriginModelName
	video.Seconds = responseSeconds(c)
	video.Size = responseSize(c)
	if response.VideoURL != "" {
		video.Status = dto.VideoStatusCompleted
		video.Progress = 100
		video.CompletedAt = time.Now().Unix()
		video.SetMetadata("url", response.VideoURL)
	}
	if response.Status == "failed" && response.Error != nil {
		video.Status = dto.VideoStatusFailed
		video.Error = &dto.OpenAIVideoError{Code: response.Error.Code, Message: response.Error.Message}
	}

	c.JSON(http.StatusOK, video)
	return upstreamTaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID), nil)
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

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	response, err := parseResponsePayload(respBody)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := &relaycommon.TaskInfo{Code: 0}
	switch response.Status {
	case "pending", "queued", "PENDING", "QUEUED":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = taskcommon.ProgressQueued
	case "processing", "running", "in_progress", "PROCESSING", "RUNNING", "IN_PROGRESS":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = taskcommon.ProgressInProgress
	case "completed", "succeeded", "success", "COMPLETED", "SUCCEEDED", "SUCCESS":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = taskcommon.ProgressComplete
		taskResult.Url = response.VideoURL
	case "failed", "cancelled", "canceled", "FAILED", "CANCELLED", "CANCELED":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = taskcommon.ProgressComplete
		if response.Error != nil && response.Error.Message != "" {
			taskResult.Reason = response.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		if response.VideoURL != "" {
			taskResult.Status = model.TaskStatusSuccess
			taskResult.Progress = taskcommon.ProgressComplete
			taskResult.Url = response.VideoURL
		} else {
			taskResult.Status = model.TaskStatusInProgress
			taskResult.Progress = taskcommon.ProgressInProgress
		}
	}
	return taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	response, err := parseResponsePayload(originTask.Data)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal aicost task data failed")
	}

	video := dto.NewOpenAIVideo()
	video.ID = originTask.TaskID
	video.TaskID = originTask.TaskID
	video.Status = originTask.Status.ToVideoStatus()
	video.SetProgressStr(originTask.Progress)
	video.CreatedAt = originTask.CreatedAt
	video.CompletedAt = originTask.UpdatedAt
	video.Model = originTask.Properties.OriginModelName
	if response.VideoURL != "" {
		video.SetMetadata("url", response.VideoURL)
	}
	if response.Error != nil {
		video.Error = &dto.OpenAIVideoError{Code: response.Error.Code, Message: response.Error.Message}
	}
	return common.Marshal(video)
}

func parseResponsePayload(data []byte) (responsePayload, error) {
	var envelope responseEnvelope
	if err := common.Unmarshal(data, &envelope); err != nil {
		return responsePayload{}, err
	}
	if envelope.Data != nil && shouldUseEnvelopeData(envelope.responsePayload, *envelope.Data) {
		return *envelope.Data, nil
	}
	if hasResponsePayloadFields(envelope.responsePayload) {
		return envelope.responsePayload, nil
	}
	if envelope.Data != nil {
		return *envelope.Data, nil
	}
	return envelope.responsePayload, nil
}

func shouldUseEnvelopeData(topLevel responsePayload, data responsePayload) bool {
	if !hasResponsePayloadFields(data) {
		return false
	}
	if data.VideoURL != "" && topLevel.VideoURL == "" {
		return true
	}
	if topLevel.Status == "" && data.Status != "" {
		return true
	}
	if topLevel.Error == nil && data.Error != nil {
		return true
	}
	if firstNonEmpty(topLevel.ID, topLevel.TaskID) == "" && firstNonEmpty(data.ID, data.TaskID) != "" {
		return true
	}
	return !hasResponsePayloadFields(topLevel)
}

func hasResponsePayloadFields(payload responsePayload) bool {
	return firstNonEmpty(payload.ID, payload.TaskID, payload.Status, payload.VideoURL) != "" || payload.Error != nil
}

func normalizeImages(req submitRequest) []string {
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

func normalizeSeconds(seconds string) string {
	if strings.TrimSpace(seconds) == "" {
		return "6"
	}
	return strings.TrimSpace(seconds)
}

func isSupportedSeconds(seconds string) bool {
	value, err := strconv.Atoi(seconds)
	return err == nil && value >= 1 && value <= 15
}

func normalizeSize(size string) string {
	if strings.TrimSpace(size) == "" {
		return "1280x720"
	}
	return strings.TrimSpace(size)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func responseSeconds(c *gin.Context) string {
	payload, ok := c.Get("aicost_task_request")
	if !ok {
		return ""
	}
	if req, ok := payload.(requestPayload); ok {
		return req.Seconds
	}
	return ""
}

func responseSize(c *gin.Context) string {
	payload, ok := c.Get("aicost_task_request")
	if !ok {
		return ""
	}
	if req, ok := payload.(requestPayload); ok {
		return req.Size
	}
	return ""
}
