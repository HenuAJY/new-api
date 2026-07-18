package moyin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoyinBuildRequestUsesMappedModelAndDocumentedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"S-MOYIN","prompt":"generate","seconds":10,"aspect_ratio":"9:16",
		"resolution":"720p","images":["https://cdn.example/i.png"],
		"videos":["https://cdn.example/v.mp4"],"audios":["https://cdn.example/a.mp3"]}`))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &relaycommon.RelayInfo{
		OriginModelName: "S-MOYIN",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://moyin.example/api", ApiKey: "sk-test"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	info.UpstreamModelName = "provider-seedance-2"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"provider-seedance-2","prompt":"generate","seconds":"10",
		"aspect_ratio":"9:16","resolution":"720p","images":["https://cdn.example/i.png"],
		"videos":["https://cdn.example/v.mp4"],"audios":["https://cdn.example/a.mp3"]}`, string(body))
}

func TestMoyinBuildRequestIgnoresUnsupportedOfficialFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"S-MOYIN","prompt":"generate","seconds":"10",
		"images":["https://cdn.example/first.png"],
		"content":[{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"}],
		"callback_url":"https://client.example/callback","return_last_frame":true,
		"service_tier":"default","execution_expires_after":3600,
		"generate_audio":false,"draft":false,"tools":[{"type":"web_search"}],
		"safety_identifier":"user_hash","priority":5,"frames":121,
		"seed":1,"camera_fixed":false,"watermark":true}`))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &relaycommon.RelayInfo{
		OriginModelName: "S-MOYIN",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://moyin.example/api", ApiKey: "sk-test"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	info.UpstreamModelName = "provider-seedance-2"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"provider-seedance-2","prompt":"generate","seconds":"10",
		"images":["https://cdn.example/first.png"]}`, string(body))
}

func TestMoyinDoResponseReturnsSeedancePublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("seedance_api", true)
	response := &http.Response{Body: io.NopCloser(strings.NewReader(`{
		"id":"upstream-task","model":"provider-seedance-2","status":"queued"}`))}
	info := &relaycommon.RelayInfo{OriginModelName: "S-MOYIN", TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}
	upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-task", upstreamID)
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestMoyinParseCompletedTaskRequiresAndReturnsVideoURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"id":"upstream-task","status":"completed","progress":100,
		"result":{"video_url":"https://cdn.example/result.mp4"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://cdn.example/result.mp4", result.Url)
	result, err = adaptor.ParseTaskResult([]byte(`{"id":"upstream-task","status":"completed","progress":100}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Empty(t, result.Url)
}

func TestMoyinConvertToSeedanceUsesStoredUpstreamURL(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusSuccess, CreatedAt: 100, UpdatedAt: 200,
		Data:        []byte(`{"status":"completed","seconds":"10","size":"9:16"}`),
		PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.example/result.mp4"},
	}
	task.Properties.OriginModelName = "S-MOYIN"
	body, err := (&TaskAdaptor{}).ConvertToSeedanceVideo(task)
	require.NoError(t, err)
	var response dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "S-MOYIN", response.Model)
	assert.Equal(t, "succeeded", response.Status)
	assert.Equal(t, "https://cdn.example/result.mp4", response.Content.VideoURL)
	require.NotNil(t, response.Duration)
	assert.Equal(t, 10, *response.Duration)
}

func TestMoyinFailureResponseDoesNotExposeUpstreamMessage(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusFailure,
		FailReason: "provider moderation details must stay private",
		Data:       []byte(`{"status":"failed","error":{"code":"generation_failed","message":"provider internal message"}}`),
	}
	task.Properties.OriginModelName = "S-MOYIN"

	body, err := (&TaskAdaptor{}).ConvertToSeedanceVideo(task)
	require.NoError(t, err)
	var response dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &response))
	require.NotNil(t, response.Error)
	assert.Equal(t, "generation_failed", response.Error.Code)
	assert.Equal(t, publicFailureMessage, response.Error.Message)
	assert.NotContains(t, string(body), "provider moderation details")
	assert.NotContains(t, string(body), "provider internal message")
}
