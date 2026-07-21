package viraldance933m

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestMapsOfficialSeedanceContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"viraldance933m","prompt":"follow the references","duration":10,
		"aspect_ratio":"9:16","resolution":"720p",
		"content":[
			{"type":"text","text":"follow the references"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/reference.png"},"role":"reference_image"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/video.mp4"},"role":"reference_video"},
			{"type":"audio_url","audio_url":{"url":"https://cdn.example/audio.mp3"},"role":"reference_audio"}
		],
		"callback_url":"https://client.example/callback","return_last_frame":true,
		"generate_audio":false,"seed":123,"watermark":true
	}`))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := testRelayInfo()
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"viraldance933m","prompt":"follow the references","duration":10,
		"aspect_ratio":"9:16","resolution":"720p",
		"start_image_url":"https://cdn.example/first.png",
		"end_image_url":"https://cdn.example/last.png",
		"image_urls":["https://cdn.example/reference.png"],
		"video_reference":[{"url":"https://cdn.example/video.mp4"}],
		"audio_reference":[{"url":"https://cdn.example/audio.mp3"}]
	}`, string(body))
	assert.NotContains(t, string(body), "callback_url")
	assert.NotContains(t, string(body), "return_last_frame")
	assert.NotContains(t, string(body), "generate_audio")
	assert.NotContains(t, string(body), "watermark")
}

func TestBuildRequestDoesNotValidateDownstreamModelAndAlwaysUsesFixedUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"channel-routing-model","prompt":"generate","seconds":"5"
	}`))
	context.Request.Header.Set("Content-Type", "application/json")
	info := testRelayInfo()
	info.ChannelBaseUrl = ""
	info.UpstreamModelName = "provider-alias-must-not-be-used"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.viralee.top/v1/videos", requestURL)
	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"viraldance933m","prompt":"generate","duration":5}`, string(body))
	assert.Equal(t, []string{"viraldance933m"}, adaptor.GetModelList())
}

func TestValidateRequestEnforcesProviderLimits(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"duration too short", `{"model":"viraldance933m","prompt":"generate","duration":3}`},
		{"too many images", `{"model":"viraldance933m","prompt":"generate","images":["1","2","3","4","5","6","7","8","9","10"]}`},
		{"too many videos", `{"model":"viraldance933m","prompt":"generate","videos":["1","2","3","4"]}`},
		{"too many audios", `{"model":"viraldance933m","prompt":"generate","audios":["1","2","3","4"]}`},
		{"inline video", `{"model":"viraldance933m","prompt":"generate","videos":["data:video/mp4;base64,AAAA"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tt.body))
			info := testRelayInfo()
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			assert.NotNil(t, adaptor.ValidateRequestAndSetAction(context, info))
		})
	}
}

func TestDoResponseReturnsPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("seedance_api", true)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"upstream-task","status":"queued"}`)),
	}
	info := testRelayInfo()
	info.PublicTaskID = "task_public"
	upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-task", upstreamID)
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestDoResponseDoesNotExposeUpstreamFailureMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body: io.NopCloser(strings.NewReader(`{
			"error":{"code":"provider_rejected","message":"private provider moderation details"}
		}`)),
	}
	_, _, taskErr := (&TaskAdaptor{}).DoResponse(context, response, testRelayInfo())
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "provider_rejected", taskErr.Code)
	assert.Equal(t, publicFailureMessage, taskErr.Message)
}

func TestParseTaskResultRequiresMetadataURLForCompletion(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"status":"completed","progress":100,
		"metadata":{"url":"https://cdn.example/result.mp4"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, taskcommon.ProgressComplete, result.Progress)
	assert.Equal(t, "https://cdn.example/result.mp4", result.Url)

	result, err = adaptor.ParseTaskResult([]byte(`{"status":"completed","progress":100}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Equal(t, taskcommon.ProgressInProgress, result.Progress)
	assert.Empty(t, result.Url)
}

func TestParseProcessingTaskClampsCompleteProgress(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"status":"processing","progress":100
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Equal(t, "99%", result.Progress)
}

func TestConvertToSeedanceUsesStoredUpstreamURLAndSafeFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}
	successTask := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusSuccess, CreatedAt: 100, UpdatedAt: 200,
		PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.example/result.mp4"},
	}
	successTask.Properties.OriginModelName = ModelName
	body, err := adaptor.ConvertToSeedanceVideo(successTask)
	require.NoError(t, err)
	var success dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &success))
	assert.Equal(t, "succeeded", success.Status)
	assert.Equal(t, "https://cdn.example/result.mp4", success.Content.VideoURL)

	failureTask := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusFailure,
		FailReason: "private poll failure", Data: []byte(`{
			"status":"failed","error":{"code":"generation_failed","message":"private upstream message"}
		}`),
	}
	failureTask.Properties.OriginModelName = ModelName
	body, err = adaptor.ConvertToSeedanceVideo(failureTask)
	require.NoError(t, err)
	var failure dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &failure))
	require.NotNil(t, failure.Error)
	assert.Equal(t, "generation_failed", failure.Error.Code)
	assert.Equal(t, publicFailureMessage, failure.Error.Message)
	assert.NotContains(t, string(body), "private")
}

func testRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: ModelName,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: defaultBaseURL,
			ApiKey:         "sk-test",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}
