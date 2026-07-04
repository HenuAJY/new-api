package apimart

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIMartBuildsGrokImagineVideoRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"grok-imagine-1.5-video-6s",
		"prompt":"walk forward",
		"input_reference":{"image_url":"https://cdn.example/scene.png"},
		"seconds":"12",
		"size":"720x1280"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.apib.ai", ApiKey: "sk-test"},
	})
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(bodyBytes, &payload))
	assert.Equal(t, "grok-imagine-1.5-video-apimart", payload["model"])
	assert.Equal(t, "walk forward", payload["prompt"])
	assert.Equal(t, float64(1), payload["duration"])
	assert.Equal(t, "9:16", payload["size"])
	assert.Equal(t, []any{"https://cdn.example/scene.png"}, payload["image_urls"])
	assert.Nil(t, adaptor.EstimateBilling(context, info))
}

func TestAPIMartRejectsBase64ReferenceImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"grok-imagine-1.5-video-6s",
		"prompt":"walk forward",
		"input_reference":{"image_url":"data:image/png;base64,AAAA"}
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(context, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}})

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "unsupported_reference_image", taskErr.Code)
}

func TestAPIMartDoResponseReturnsPublicOpenAIVideoForAsyncTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":200,"data":[{"task_id":"upstream_task","status":"submitted"}]}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: DownstreamModel,
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(context, response, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_task", upstreamTaskID)
	assert.JSONEq(t, `{"code":200,"data":[{"task_id":"upstream_task","status":"submitted"}]}`, string(taskData))
	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &video))
	assert.Equal(t, "task_public", video.ID)
	assert.Equal(t, DownstreamModel, video.Model)
	assert.Equal(t, dto.VideoStatusQueued, video.Status)
}

func TestAPIMartParseTaskResultUsesProgressAndVideoURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"data":{"id":"upstream_task","status":"processing","progress":67,"result":{"videos":[{"url":["https://cdn.example/video.mp4"],"expires_at":1763174708}]}}}`)

	taskInfo, err := adaptor.ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, taskInfo.Status)
	assert.Equal(t, "67%", taskInfo.Progress)
	assert.Equal(t, "https://cdn.example/video.mp4", taskInfo.Url)

	completedBody := []byte(`{"code":200,"data":{"id":"upstream_task","status":"completed","progress":100,"result":{"videos":[{"url":"https://cdn.example/video.mp4"}]}}}`)
	taskInfo, err = adaptor.ParseTaskResult(completedBody)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	assert.Equal(t, "100%", taskInfo.Progress)
	assert.Equal(t, "https://cdn.example/video.mp4", taskInfo.Url)
}

func TestAPIMartParseTaskResultSupportsRealCompletedResponse(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"data":{"actual_time":42,"completed":1783170633,"cost":0.008,"created":1783170591,"credits_cost":0.08,"estimated_time":100,"id":"task_01KWPKZTQNM84TDK68C3990FR7","progress":100,"result":{"videos":[{"expires_at":1783257033,"url":["https://getapib.org/video/491d8559-7f65-4a51-bc4a-ca875eab6e38.mp4"]}]},"status":"completed"}}`)

	taskInfo, err := adaptor.ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	assert.Equal(t, "100%", taskInfo.Progress)
	assert.Equal(t, "https://getapib.org/video/491d8559-7f65-4a51-bc4a-ca875eab6e38.mp4", taskInfo.Url)
	assert.Equal(t, int64(1783170591), taskInfo.StartTime)
	assert.Equal(t, int64(1783170633), taskInfo.FinishTime)
}

func TestAPIMartBuildRequestURLAndHeaders(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.apib.ai", ApiKey: "sk-test"},
	})

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{})
	require.NoError(t, err)
	assert.Equal(t, "https://api.apib.ai/v1/videos/generations", url)

	request := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	require.NoError(t, adaptor.BuildRequestHeader(nil, request, &relaycommon.RelayInfo{}))
	assert.Equal(t, "Bearer sk-test", request.Header.Get("Authorization"))
	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
}

func TestAPIMartModelListContainsOnlyDownstreamModel(t *testing.T) {
	assert.Equal(t, []string{DownstreamModel}, (&TaskAdaptor{}).GetModelList())
	assert.Equal(t, "APIMart", (&TaskAdaptor{}).GetChannelName())
	assert.Contains(t, ModelList, DownstreamModel)
	assert.Equal(t, constant.ChannelTypeAPIMart, constant.ChannelTypeAPIMart)
}
