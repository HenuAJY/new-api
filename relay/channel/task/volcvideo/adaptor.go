package volcvideo

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
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
)

type taskContent struct {
	VideoURL     string `json:"video_url,omitempty"`
	LastFrameURL string `json:"last_frame_url,omitempty"`
}

type taskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type taskUsage struct {
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	ToolUsage        struct {
		WebSearch int `json:"web_search,omitempty"`
	} `json:"tool_usage,omitempty"`
}

type taskResponse struct {
	ID                    string                 `json:"id"`
	Model                 string                 `json:"model"`
	Status                string                 `json:"status"`
	Error                 *taskError             `json:"error"`
	CreatedAt             int64                  `json:"created_at"`
	UpdatedAt             int64                  `json:"updated_at"`
	Content               taskContent            `json:"content"`
	Seed                  *int                   `json:"seed,omitempty"`
	Resolution            string                 `json:"resolution,omitempty"`
	Ratio                 string                 `json:"ratio,omitempty"`
	Duration              *int                   `json:"duration,omitempty"`
	Frames                *int                   `json:"frames,omitempty"`
	FramesPerSecond       *int                   `json:"framespersecond,omitempty"`
	GenerateAudio         *bool                  `json:"generate_audio,omitempty"`
	Tools                 []taskdto.SeedanceTool `json:"tools,omitempty"`
	SafetyIdentifier      string                 `json:"safety_identifier,omitempty"`
	Priority              *int                   `json:"priority,omitempty"`
	Draft                 *bool                  `json:"draft,omitempty"`
	DraftTaskID           string                 `json:"draft_task_id,omitempty"`
	ServiceTier           string                 `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *int                   `json:"execution_expires_after,omitempty"`
	Usage                 *taskUsage             `json:"usage,omitempty"`
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

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	var request taskdto.SeedanceTaskRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model is required"), "missing_model", http.StatusBadRequest)
	}
	if len(request.Content) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("content is required"), "invalid_request", http.StatusBadRequest)
	}

	info.Action = constant.TaskActionGenerate
	c.Set("volcvideo_task_request", request)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt: firstText(request.Content), Model: request.Model,
		Duration: valueOrZero(request.Duration), Metadata: map[string]any{"content": request.Content, "resolution": request.Resolution},
	})
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/api/v3/contents/generations/tasks", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, request *http.Request, _ *relaycommon.RelayInfo) error {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	value, ok := c.Get("volcvideo_task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	request, ok := value.(taskdto.SeedanceTaskRequest)
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

func (a *TaskAdaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (string, []byte, *taskdto.TaskError) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = response.Body.Close()
	var result struct {
		ID string `json:"id"`
	}
	if err := common.Unmarshal(body, &result); err != nil || result.ID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf(publicFailureMessage), "invalid_upstream_response", http.StatusBadGateway)
	}
	if c.GetBool("seedance_api") {
		c.JSON(http.StatusOK, taskdto.SeedanceCreateResponse{ID: info.PublicTaskID})
	} else {
		video := dto.NewOpenAIVideo()
		video.ID = info.PublicTaskID
		video.TaskID = info.PublicTaskID
		video.Model = info.OriginModelName
		video.CreatedAt = time.Now().Unix()
		c.JSON(http.StatusOK, video)
	}
	return result.ID, body, nil
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
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v3/contents/generations/tasks/"+taskID, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	var response taskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	result := &relaycommon.TaskInfo{TaskID: response.ID}
	switch response.Status {
	case "queued":
		result.Status, result.Progress = model.TaskStatusQueued, "10%"
	case "running":
		result.Status, result.Progress = model.TaskStatusInProgress, "50%"
	case "succeeded":
		result.Status, result.Progress, result.Url = model.TaskStatusSuccess, "100%", response.Content.VideoURL
	case "failed", "cancelled", "expired":
		result.Status, result.Progress = model.TaskStatusFailure, "100%"
		if response.Error != nil {
			result.Reason = response.Error.Message
		}
	default:
		result.Status = model.TaskStatusInProgress
	}
	if response.Usage != nil {
		result.CompletionTokens = response.Usage.CompletionTokens
		result.TotalTokens = response.Usage.TotalTokens
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertToSeedanceVideo(task *model.Task) ([]byte, error) {
	var response taskResponse
	_ = common.Unmarshal(task.Data, &response)
	response.ID = task.TaskID
	response.Model = task.Properties.OriginModelName
	response.Status = toSeedanceStatus(task.Status)
	response.CreatedAt = task.CreatedAt
	response.UpdatedAt = task.UpdatedAt
	response.Error = nil
	if task.Status == model.TaskStatusSuccess {
		response.Content.VideoURL = task.GetResultURL()
	}
	if task.Status == model.TaskStatusFailure {
		response.Content = taskContent{}
		response.Error = &taskError{Message: publicFailureMessage}
		var upstream taskResponse
		if common.Unmarshal(task.Data, &upstream) == nil && upstream.Error != nil {
			response.Error.Code = upstream.Error.Code
		}
	}
	return common.Marshal(response)
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := task.ToOpenAIVideo()
	video.TaskID = task.TaskID
	if task.Status == model.TaskStatusFailure {
		video.Metadata = nil
		video.Error = &dto.OpenAIVideoError{Message: publicFailureMessage}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func firstText(content []taskdto.SeedanceContentItem) string {
	for _, item := range content {
		if item.Type == "text" {
			return strings.TrimSpace(item.Text)
		}
	}
	return ""
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
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
