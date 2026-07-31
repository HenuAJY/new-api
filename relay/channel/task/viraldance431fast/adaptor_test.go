package viraldance431fast

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestMapsOfficialSeedanceContent(t *testing.T) {
	context := testContext(`{
		"model":"downstream-fast-model","prompt":"follow the references","duration":15,
		"aspect_ratio":"9:16","size":"720x1280","resolution":"720p",
		"content":[
			{"type":"text","text":"follow the references"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/reference.png"},"role":"reference_image"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/video.mp4"},"role":"reference_video"},
			{"type":"audio_url","audio_url":{"url":"https://cdn.example/audio.mp3"},"role":"reference_audio"}
		],
		"return_last_frame":true,"generate_audio":false,"seed":123,"watermark":true
	}`)
	info := testRelayInfo()
	info.OriginModelName = "downstream-fast-model"
	info.UpstreamModelName = "provider-custom-fast-model"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"provider-custom-fast-model","prompt":"follow the references","duration":15,
		"aspect_ratio":"9:16","size":"720x1280","resolution":"720p",
		"start_image_url":"https://cdn.example/first.png",
		"end_image_url":"https://cdn.example/last.png",
		"image_urls":["https://cdn.example/reference.png"],
		"video_reference":[{"url":"https://cdn.example/video.mp4"}],
		"audio_reference":[{"url":"https://cdn.example/audio.mp3"}]
	}`, string(body))
	assert.NotContains(t, string(body), "return_last_frame")
	assert.NotContains(t, string(body), "generate_audio")
	assert.NotContains(t, string(body), "watermark")
	assert.Nil(t, adaptor.EstimateBilling(context, info))
}

func TestBuildRequestMapsOfficialContentPromptAndRatio(t *testing.T) {
	context := testContext(`{
		"model":"hjsv-viraldance431-fast-480p",
		"content":[
			{"type":"text","text":"图1人物和图2人物在KTV唱歌"},
			{"type":"image_url","image_url":{"url":"https://naoos.tos-cn-shanghai.volces.com/%E7%9C%9F%E4%BA%BA/1f8a65daf8174f79ace65d49f2266b57.png"},"role":"reference_image"},
			{"type":"image_url","image_url":{"url":"https://naoos.tos-cn-shanghai.volces.com/%E7%9C%9F%E4%BA%BA/1cb0bf847d7a46c0915965d1418ffe33.png"},"role":"reference_image"}
		],
		"resolution":"720p","ratio":"16:9","duration":4,
		"generate_audio":true,"watermark":false,"priority":0
	}`)
	info := testRelayInfo()
	info.OriginModelName = "hjsv-viraldance431-fast-480p"
	info.UpstreamModelName = "viraldance431-fast"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"viraldance431-fast","prompt":"图1人物和图2人物在KTV唱歌",
		"duration":4,"aspect_ratio":"16:9","resolution":"720p",
		"image_urls":[
			"https://naoos.tos-cn-shanghai.volces.com/%E7%9C%9F%E4%BA%BA/1f8a65daf8174f79ace65d49f2266b57.png",
			"https://naoos.tos-cn-shanghai.volces.com/%E7%9C%9F%E4%BA%BA/1cb0bf847d7a46c0915965d1418ffe33.png"
		]
	}`, string(body))
	assert.NotContains(t, string(body), "generate_audio")
	assert.NotContains(t, string(body), "watermark")
	assert.NotContains(t, string(body), "priority")
}

func TestBuildRequestDoesNotValidateOrFixModelName(t *testing.T) {
	context := testContext(`{"model":"arbitrary-downstream-model","prompt":"generate","seconds":"5"}`)
	info := testRelayInfo()
	info.OriginModelName = "arbitrary-downstream-model"
	info.UpstreamModelName = "arbitrary-provider-model"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"arbitrary-provider-model","prompt":"generate","duration":5}`, string(body))
	assert.Equal(t, []string{"viraldance431-fast"}, adaptor.GetModelList())
}

func TestValidateRequestEnforcesProviderLimits(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"duration too short", `{"model":"any","prompt":"generate","duration":3}`},
		{"duration too long", `{"model":"any","prompt":"generate","duration":16}`},
		{"too many images", `{"model":"any","prompt":"generate","images":["1","2","3","4","5"]}`},
		{"too many videos", `{"model":"any","prompt":"generate","videos":["1","2","3","4"]}`},
		{"too many audios", `{"model":"any","prompt":"generate","audios":["1","2"]}`},
		{"inline video", `{"model":"any","prompt":"generate","videos":["data:video/mp4;base64,AAAA"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := testContext(test.body)
			info := testRelayInfo()
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			assert.NotNil(t, adaptor.ValidateRequestAndSetAction(context, info))
		})
	}
}

func TestDoResponseReturnsPublicTaskIDAndHidesUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("seedance_api", true)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"task_id":"upstream-task","status":"submitted"}`)),
	}
	info := testRelayInfo()
	info.PublicTaskID = "task_public"
	upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-task", upstreamID)
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())

	failureResponse := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body: io.NopCloser(strings.NewReader(`{
			"error":{"code":"provider_rejected","message":"private provider details"}
		}`)),
	}
	_, _, taskErr = (&TaskAdaptor{}).DoResponse(context, failureResponse, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "provider_rejected", taskErr.Code)
	assert.Equal(t, publicFailureMessage, taskErr.Message)
}

func TestParseTaskResultRequiresVideoURLForCompletion(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"status":"completed","progress":100,"url":"https://cdn.example/result.mp4"
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

	result, err = adaptor.ParseTaskResult([]byte(`{
		"status":"completed","metadata":{"url":"https://cdn.example/fallback.mp4"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/fallback.mp4", result.Url)
}

func TestParseInProgressTaskClampsCompleteProgress(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"processing","progress":100}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Equal(t, "99%", result.Progress)
}

func TestConvertToSeedanceUsesStoredURLAndSafeFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}
	successTask := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusSuccess, CreatedAt: 100, UpdatedAt: 200,
		PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.example/result.mp4"},
	}
	successTask.Properties.OriginModelName = "downstream-fast-model"
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
	failureTask.Properties.OriginModelName = "downstream-fast-model"
	body, err = adaptor.ConvertToSeedanceVideo(failureTask)
	require.NoError(t, err)
	var failure dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &failure))
	require.NotNil(t, failure.Error)
	assert.Equal(t, "generation_failed", failure.Error.Code)
	assert.Equal(t, publicFailureMessage, failure.Error.Message)
	assert.NotContains(t, string(body), "private")
}

func testContext(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	return context
}

func testRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: defaultModelName,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: defaultBaseURL,
			ApiKey:         "sk-test",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}
