package viraldance933v

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

func TestBuildRequestUsesMappedModelAndIgnoresUnsupportedExtensions(t *testing.T) {
	context := testContext(`{
		"model":"downstream-alias","prompt":"vertical video","duration":15,
		"content":[
			{"type":"text","text":"vertical video"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/ref.png"},"role":"reference_image"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/ref.mp4"}},
			{"type":"audio_url","audio_url":{"url":"https://cdn.example/ref.mp3"}}
		],
		"ratio":"16:9","resolution":"1080p","enable_face_mask":true,
		"return_last_frame":true,"seed":1,"watermark":true
	}`)
	info := testRelayInfo()
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"provider-vertical-seedance","prompt":"vertical video","duration":15,
		"start_image_url":"https://cdn.example/first.png",
		"end_image_url":"https://cdn.example/last.png",
		"image_urls":["https://cdn.example/ref.png"],
		"video_reference":[{"url":"https://cdn.example/ref.mp4"}],
		"audio_reference":[{"url":"https://cdn.example/ref.mp3"}]
	}`, string(body))
	assert.NotContains(t, string(body), "aspect_ratio")
	assert.NotContains(t, string(body), "enable_face_mask")
	assert.NotContains(t, string(body), "return_last_frame")
	assert.NotContains(t, string(body), `"seed":`)
}

func TestBuildRequestDoesNotRequireFixedModelName(t *testing.T) {
	context := testContext(`{"model":"any-model-name","prompt":"generate"}`)
	info := testRelayInfo()
	info.UpstreamModelName = ""
	info.OriginModelName = "mapped-origin-model"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"mapped-origin-model","prompt":"generate"}`, string(body))
	assert.Empty(t, adaptor.GetModelList())
}

func TestValidateRequestEnforcesViraldanceLimits(t *testing.T) {
	for _, body := range []string{
		`{"model":"x","prompt":"generate","duration":0}`,
		`{"model":"x","prompt":"generate","duration":16}`,
		`{"model":"x","prompt":"generate","images":["1","2","3","4","5","6","7","8","9","10"]}`,
		`{"model":"x","prompt":"generate","videos":["1","2","3","4"]}`,
		`{"model":"x","prompt":"generate","audios":["1","2","3","4"]}`,
		`{"model":"x","prompt":"generate","videos":["data:video/mp4;base64,AAAA"]}`,
	} {
		adaptor := &TaskAdaptor{}
		adaptor.Init(testRelayInfo())
		assert.NotNil(t, adaptor.ValidateRequestAndSetAction(testContext(body), testRelayInfo()))
	}
}

func TestParseTaskResultRequiresVideoURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"status":"completed","progress":100,"url":"https://cdn.example/video.mp4"
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", result.Url)

	result, err = adaptor.ParseTaskResult([]byte(`{
		"status":"completed","progress":100,"metadata":{"url":"https://cdn.example/metadata.mp4"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://cdn.example/metadata.mp4", result.Url)

	result, err = adaptor.ParseTaskResult([]byte(`{"status":"completed","progress":100}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Equal(t, taskcommon.ProgressInProgress, result.Progress)
}

func TestParseInProgressProgress100IsClamped(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"in_progress","progress":100}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Equal(t, "99%", result.Progress)
}

func TestDoResponseAndSeedanceConversionHideProviderMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("seedance_api", true)
	info := testRelayInfo()
	info.PublicTaskID = "task_public"
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"upstream-task","status":"queued"}`))}
	upstreamID, _, err := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, err)
	assert.Equal(t, "upstream-task", upstreamID)
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())

	task := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusFailure,
		Data: []byte(`{"status":"failed","error":{"code":"failed","message":"private provider message"}}`),
	}
	task.Properties.OriginModelName = "mapped-origin-model"
	body, marshalErr := (&TaskAdaptor{}).ConvertToSeedanceVideo(task)
	require.NoError(t, marshalErr)
	var output dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &output))
	require.NotNil(t, output.Error)
	assert.Equal(t, publicFailureMessage, output.Error.Message)
	assert.NotContains(t, string(body), "private provider message")
}

func testContext(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	return context
}

func testRelayInfo() *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		OriginModelName: "downstream-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: defaultBaseURL,
			ApiKey:         "sk-test",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	info.UpstreamModelName = "provider-vertical-seedance"
	return info
}
