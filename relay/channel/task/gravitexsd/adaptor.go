package gravitexsd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	Model         string                    `json:"model"`
	Prompt        string                    `json:"prompt,omitempty"`
	Content       []dto.SeedanceContentItem `json:"content,omitempty"`
	Duration      *dto.IntValue             `json:"duration,omitempty"`
	Seconds       *dto.IntValue             `json:"seconds,omitempty"`
	Resolution    string                    `json:"resolution,omitempty"`
	Ratio         string                    `json:"ratio,omitempty"`
	GenerateAudio *dto.BoolValue            `json:"generate_audio,omitempty"`
	Watermark     *dto.BoolValue            `json:"watermark,omitempty"`
	Seed          *dto.IntValue             `json:"seed,omitempty"`
	Images        []string                  `json:"images,omitempty"`
	Videos        []string                  `json:"videos,omitempty"`
	Audios        []string                  `json:"audios,omitempty"`
}

type upstreamRequest struct {
	Model         string                    `json:"model"`
	Prompt        string                    `json:"prompt,omitempty"`
	Content       []dto.SeedanceContentItem `json:"content,omitempty"`
	Duration      *dto.IntValue             `json:"duration,omitempty"`
	Resolution    string                    `json:"resolution,omitempty"`
	Ratio         string                    `json:"ratio,omitempty"`
	GenerateAudio *dto.BoolValue            `json:"generate_audio,omitempty"`
	Watermark     *dto.BoolValue            `json:"watermark,omitempty"`
	Seed          *dto.IntValue             `json:"seed,omitempty"`
}

type taskResponse struct {
	Code        string        `json:"code,omitempty"`
	Message     string        `json:"message,omitempty"`
	Data        *taskResponse `json:"data,omitempty"`
	ID          string        `json:"id,omitempty"`
	TaskID      string        `json:"task_id,omitempty"`
	Model       string        `json:"model,omitempty"`
	Status      string        `json:"status,omitempty"`
	Progress    int           `json:"progress,omitempty"`
	CreatedAt   int64         `json:"created_at,omitempty"`
	CompletedAt int64         `json:"completed_at,omitempty"`
	FailedAt    int64         `json:"failed_at,omitempty"`
	URL         string        `json:"url,omitempty"`
	VideoURL    string        `json:"video_url,omitempty"`
	Metadata    taskMetadata  `json:"metadata,omitempty"`
	Error       *taskError    `json:"error,omitempty"`
}

type taskMetadata struct {
	URL      string              `json:"url,omitempty"`
	VideoURL string              `json:"video_url,omitempty"`
	Content  taskMetadataContent `json:"content,omitempty"`
}

type taskMetadataContent struct {
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

var (
	assetGroupCache   sync.Map
	assetGroupCacheMu sync.Mutex
	assetURLCache     sync.Map
)

type assetGroupListResponse struct {
	Groups []assetGroup `json:"groups"`
}

type assetGroup struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
}

type createAssetResponse struct {
	VirtualID string `json:"virtual_id"`
	AssetURL  string `json:"asset_url"`
	Status    string `json:"status"`
}

type assetStatusResponse struct {
	VirtualID string `json:"virtual_id"`
	AssetURL  string `json:"asset_url"`
	Status    string `json:"status"`
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

	upstream := upstreamRequest{
		Prompt:        strings.TrimSpace(request.Prompt),
		Content:       supportedContent(request.Content),
		GenerateAudio: request.GenerateAudio,
		Watermark:     request.Watermark,
		Seed:          request.Seed,
	}
	if len(upstream.Content) == 0 {
		upstream.Content = legacyContent(request.Images, request.Videos, request.Audios)
	}
	if len(upstream.Content) > 0 {
		upstream.Prompt = ""
	}
	if upstream.Prompt == "" && len(upstream.Content) == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt or content is required"), "invalid_request", http.StatusBadRequest)
	}
	if len([]rune(strings.TrimSpace(request.Prompt))) > 5000 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt must not exceed 5000 characters"), "invalid_request", http.StatusBadRequest)
	}

	upstream.Duration = request.Duration
	if upstream.Duration == nil {
		upstream.Duration = request.Seconds
	}
	if upstream.Duration != nil && int(*upstream.Duration) != -1 && (int(*upstream.Duration) < 4 || int(*upstream.Duration) > 15) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration must be -1 or between 4 and 15 seconds"), "invalid_request", http.StatusBadRequest)
	}
	if isSupportedResolution(request.Resolution) {
		upstream.Resolution = request.Resolution
	}
	if isSupportedRatio(request.Ratio) {
		upstream.Ratio = request.Ratio
	}
	if taskErr := validateContent(upstream.Content); taskErr != nil {
		return taskErr
	}

	info.Action = constant.TaskActionGenerate
	c.Set("gravitexsd_task_request", upstream)
	seconds := ""
	if upstream.Duration != nil {
		seconds = strconv.Itoa(int(*upstream.Duration))
	}
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:  firstText(upstream.Content, strings.TrimSpace(request.Prompt)),
		Model:   request.Model,
		Images:  contentImageURLs(upstream.Content),
		Seconds: seconds,
		Size:    upstream.Ratio,
	})
	return nil
}

// EstimateBilling applies the configured per-second model price to the requested duration.
// Automatic duration has no actual duration in the upstream response, so its documented
// five-second default is used for both pre-charge and final task accounting.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	value, ok := c.Get("gravitexsd_task_request")
	if !ok {
		return nil
	}
	request, ok := value.(upstreamRequest)
	if !ok {
		return nil
	}
	duration := defaultDuration
	if request.Duration != nil && int(*request.Duration) > 0 {
		duration = int(*request.Duration)
	}
	return map[string]float64{"seconds": float64(duration)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/video/generations", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, request *http.Request, _ *relaycommon.RelayInfo) error {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	value, ok := c.Get("gravitexsd_task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	request, ok := value.(upstreamRequest)
	if !ok {
		return nil, fmt.Errorf("invalid task request type")
	}
	preparedContent, err := a.prepareAssetContent(c, info, request.Content)
	if err != nil {
		return nil, err
	}
	request.Content = preparedContent
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

func (a *TaskAdaptor) prepareAssetContent(c *gin.Context, info *relaycommon.RelayInfo, content []dto.SeedanceContentItem) ([]dto.SeedanceContentItem, error) {
	if len(content) == 0 {
		return content, nil
	}
	groupID, err := a.ensureAssetGroup(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	prepared := make([]dto.SeedanceContentItem, len(content))
	copy(prepared, content)
	for i := range prepared {
		item := &prepared[i]
		assetType, rawURL := contentAssetTypeAndURL(*item)
		if assetType == "" {
			continue
		}
		assetURL, err := requirePublicURL(rawURL)
		if err != nil {
			return nil, err
		}
		assetURL, err = a.ensureAsset(c, info.ChannelSetting.Proxy, groupID, assetType, assetURL)
		if err != nil {
			return nil, err
		}
		setContentAssetURL(item, assetURL)
	}
	return prepared, nil
}

func (a *TaskAdaptor) ensureAssetGroup(proxy string) (string, error) {
	cacheKey := a.baseURL + "\x00" + a.apiKey
	if value, ok := assetGroupCache.Load(cacheKey); ok {
		return value.(string), nil
	}
	assetGroupCacheMu.Lock()
	defer assetGroupCacheMu.Unlock()
	if value, ok := assetGroupCache.Load(cacheKey); ok {
		return value.(string), nil
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequest(http.MethodGet, a.baseURL+"/v1/asset-groups?group_type=aigc", nil)
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr == nil && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		var groups assetGroupListResponse
		if common.Unmarshal(body, &groups) == nil {
			for _, group := range groups.Groups {
				if group.Name == managedAssetGroupName && strings.TrimSpace(group.GroupID) != "" {
					assetGroupCache.Store(cacheKey, group.GroupID)
					return group.GroupID, nil
				}
			}
		}
	}

	payload, err := common.Marshal(map[string]string{
		"name":        managedAssetGroupName,
		"description": "managed by NewAPI",
		"group_type":  "aigc",
	})
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	request, err = http.NewRequest(http.MethodPost, a.baseURL+"/v1/asset-groups", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	response, err = client.Do(request)
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	body, readErr = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	var group assetGroup
	if common.Unmarshal(body, &group) != nil || strings.TrimSpace(group.GroupID) == "" {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	assetGroupCache.Store(cacheKey, group.GroupID)
	return group.GroupID, nil
}

func (a *TaskAdaptor) ensureAsset(c *gin.Context, proxy, groupID, assetType, sourceURL string) (string, error) {
	cacheKey := a.baseURL + "\x00" + a.apiKey + "\x00" + assetType + "\x00" + sourceURL
	if value, ok := assetURLCache.Load(cacheKey); ok {
		return value.(string), nil
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	if client == nil {
		client = http.DefaultClient
	}
	payload, err := common.Marshal(map[string]string{
		"url":        sourceURL,
		"group_id":   groupID,
		"asset_type": assetType,
	})
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, a.baseURL+"/v1/assets", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	var created createAssetResponse
	if common.Unmarshal(body, &created) != nil || strings.TrimSpace(created.VirtualID) == "" {
		return "", fmt.Errorf("prepare reference assets failed")
	}
	assetURL := strings.TrimSpace(created.AssetURL)
	if strings.EqualFold(created.Status, "active") && assetURL != "" {
		assetURLCache.Store(cacheKey, assetURL)
		return assetURL, nil
	}
	if assetURL == "" {
		assetURL, err = a.waitForAsset(c, client, created.VirtualID)
	} else {
		assetURL, err = a.waitForAsset(c, client, created.VirtualID)
	}
	if err != nil {
		return "", err
	}
	assetURLCache.Store(cacheKey, assetURL)
	return assetURL, nil
}

func (a *TaskAdaptor) waitForAsset(c *gin.Context, client *http.Client, virtualID string) (string, error) {
	deadline := time.Now().Add(3 * time.Minute)
	for {
		request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, a.baseURL+"/v1/assets/"+virtualID, nil)
		if err != nil {
			return "", fmt.Errorf("prepare reference assets failed")
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+a.apiKey)
		response, err := client.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
				var status assetStatusResponse
				if common.Unmarshal(body, &status) == nil {
					switch strings.ToLower(status.Status) {
					case "active":
						if strings.TrimSpace(status.AssetURL) != "" {
							return status.AssetURL, nil
						}
					case "failed":
						return "", fmt.Errorf("prepare reference assets failed")
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("prepare reference assets failed")
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			return "", fmt.Errorf("prepare reference assets failed")
		case <-timer.C:
		}
	}
}

func contentAssetTypeAndURL(item dto.SeedanceContentItem) (string, string) {
	switch item.Type {
	case "image_url":
		if item.ImageURL != nil {
			return "Image", item.ImageURL.URL
		}
	case "video_url":
		if item.VideoURL != nil {
			return "Video", item.VideoURL.URL
		}
	case "audio_url":
		if item.AudioURL != nil {
			return "Audio", item.AudioURL.URL
		}
	}
	return "", ""
}

func setContentAssetURL(item *dto.SeedanceContentItem, assetURL string) {
	switch item.Type {
	case "image_url":
		if item.ImageURL != nil {
			item.ImageURL.URL = assetURL
		}
	case "video_url":
		if item.VideoURL != nil {
			item.VideoURL.URL = assetURL
		}
	case "audio_url":
		if item.AudioURL != nil {
			item.AudioURL.URL = assetURL
		}
	}
}

func requirePublicURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("reference media must use public http or https URLs")
	}
	return value, nil
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

	result, err := decodeTaskResponse(body)
	if err != nil {
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
	request, err := http.NewRequest(http.MethodGet, baseURL+"/v1/video/generations/"+taskID, nil)
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
	response, err := decodeTaskResponse(body)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	result := &relaycommon.TaskInfo{
		Code:       0,
		Url:        firstNonEmpty(response.VideoURL, response.URL, response.Metadata.VideoURL, response.Metadata.URL, response.Metadata.Content.VideoURL),
		Progress:   normalizeProgress(response.Progress),
		StartTime:  response.CreatedAt,
		FinishTime: firstPositive(response.CompletedAt, response.FailedAt),
	}
	switch strings.ToLower(response.Status) {
	case "queued", "submitted":
		result.Status = model.TaskStatusQueued
	case "in_progress", "processing", "running":
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
	response, _ := decodeTaskResponse(task.Data)
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
	response, _ := decodeTaskResponse(task.Data)
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

func validateContent(content []dto.SeedanceContentItem) *dto.TaskError {
	var textItems, firstFrames, lastFrames, referenceImages, referenceVideos, referenceAudios, visualReferences int
	for _, item := range content {
		switch item.Type {
		case "text":
			textItems++
			if strings.TrimSpace(item.Text) == "" {
				return service.TaskErrorWrapperLocal(fmt.Errorf("text content must not be empty"), "invalid_request", http.StatusBadRequest)
			}
			if len([]rune(item.Text)) > 5000 {
				return service.TaskErrorWrapperLocal(fmt.Errorf("text content must not exceed 5000 characters"), "invalid_request", http.StatusBadRequest)
			}
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				return service.TaskErrorWrapperLocal(fmt.Errorf("image_url.url is required"), "invalid_request", http.StatusBadRequest)
			}
			if _, err := requirePublicURL(item.ImageURL.URL); err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
			}
			switch item.Role {
			case "", "first_frame":
				firstFrames++
				visualReferences++
			case "last_frame":
				lastFrames++
			case "reference_image":
				referenceImages++
				visualReferences++
			default:
				referenceImages++
				visualReferences++
			}
		case "video_url":
			if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
				return service.TaskErrorWrapperLocal(fmt.Errorf("video_url.url is required"), "invalid_request", http.StatusBadRequest)
			}
			if _, err := requirePublicURL(item.VideoURL.URL); err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
			}
			referenceVideos++
			visualReferences++
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
				return service.TaskErrorWrapperLocal(fmt.Errorf("audio_url.url is required"), "invalid_request", http.StatusBadRequest)
			}
			if _, err := requirePublicURL(item.AudioURL.URL); err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
			}
			referenceAudios++
		}
	}
	if textItems > 1 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("at most 1 text content item is supported"), "invalid_request", http.StatusBadRequest)
	}
	if firstFrames > 1 || lastFrames > 1 || referenceImages > 9 || referenceVideos > 3 || referenceAudios > 3 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("reference media exceeds provider limits"), "invalid_request", http.StatusBadRequest)
	}
	if firstFrames > 0 && referenceImages > 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("first_frame and reference_image cannot be used together"), "invalid_request", http.StatusBadRequest)
	}
	if lastFrames > 0 && firstFrames == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("last_frame requires first_frame"), "invalid_request", http.StatusBadRequest)
	}
	if referenceAudios > 0 && visualReferences == 0 {
		return service.TaskErrorWrapperLocal(fmt.Errorf("audio reference requires an image or video reference"), "invalid_request", http.StatusBadRequest)
	}
	return nil
}

func supportedContent(content []dto.SeedanceContentItem) []dto.SeedanceContentItem {
	result := make([]dto.SeedanceContentItem, 0, len(content))
	for _, item := range content {
		switch item.Type {
		case "text", "image_url", "video_url", "audio_url":
			result = append(result, item)
		}
	}
	return result
}

func legacyContent(images, videos, audios []string) []dto.SeedanceContentItem {
	content := make([]dto.SeedanceContentItem, 0, len(images)+len(videos)+len(audios))
	for _, url := range images {
		if url = strings.TrimSpace(url); url != "" {
			content = append(content, dto.SeedanceContentItem{Type: "image_url", ImageURL: &dto.SeedanceMediaURL{URL: url}, Role: "reference_image"})
		}
	}
	for _, url := range videos {
		if url = strings.TrimSpace(url); url != "" {
			content = append(content, dto.SeedanceContentItem{Type: "video_url", VideoURL: &dto.SeedanceMediaURL{URL: url}, Role: "reference_video"})
		}
	}
	for _, url := range audios {
		if url = strings.TrimSpace(url); url != "" {
			content = append(content, dto.SeedanceContentItem{Type: "audio_url", AudioURL: &dto.SeedanceMediaURL{URL: url}, Role: "reference_audio"})
		}
	}
	return content
}

func contentImageURLs(content []dto.SeedanceContentItem) []string {
	urls := make([]string, 0)
	for _, item := range content {
		if item.Type == "image_url" && item.ImageURL != nil && strings.TrimSpace(item.ImageURL.URL) != "" {
			urls = append(urls, item.ImageURL.URL)
		}
	}
	return urls
}

func firstText(content []dto.SeedanceContentItem, fallback string) string {
	for _, item := range content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return item.Text
		}
	}
	return fallback
}

func isSupportedResolution(resolution string) bool {
	switch resolution {
	case "480p", "720p", "1080p":
		return true
	default:
		return false
	}
}

func isSupportedRatio(ratio string) bool {
	switch ratio {
	case "16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive":
		return true
	default:
		return false
	}
}

func toOpenAIVideoStatus(status string) string {
	switch strings.ToLower(status) {
	case "in_progress", "processing", "running":
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

func decodeTaskResponse(body []byte) (taskResponse, error) {
	var response taskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return taskResponse{}, err
	}
	if response.Data == nil {
		return response, nil
	}
	data := *response.Data
	data.Code = firstNonEmpty(data.Code, response.Code)
	data.Message = firstNonEmpty(data.Message, response.Message)
	data.Data = nil
	return data, nil
}

func responseStatus(response *http.Response) int {
	if response.StatusCode >= http.StatusBadRequest {
		return response.StatusCode
	}
	return http.StatusBadGateway
}
