package volcvideo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialCreateRequestAndResponseContract(t *testing.T) {
	context, recorder := newTaskContext(`{
		"model":"endpoint-id","content":[
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"}
		],"return_last_frame":true,"generate_audio":false,"duration":10,"ratio":"16:9","resolution":"1080p"
	}`)
	context.Set("seedance_api", true)
	info := &relaycommon.RelayInfo{
		OriginModelName: "endpoint-id",
		ChannelMeta:     &relaycommon.ChannelMeta{},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	info.UpstreamModelName = "endpoint-id"
	info.PublicTaskID = "task_public"
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	requestBody, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"endpoint-id","content":[
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"}
		],"return_last_frame":true,"generate_audio":false,"duration":10,"ratio":"16:9","resolution":"1080p"
	}`, string(body))

	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"upstream_task"}`))}
	taskID, _, taskErr := adaptor.DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_task", taskID)
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestConvertToOfficialTaskResponse(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusSuccess, CreatedAt: 100, UpdatedAt: 200,
		PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.example/video.mp4"},
		Data: []byte(`{
			"id":"upstream_task","model":"upstream-model","status":"succeeded",
			"content":{"video_url":"https://upstream.example/video.mp4","last_frame_url":"https://cdn.example/last.png"},
			"seed":42,"resolution":"1080p","ratio":"16:9","duration":10,"framespersecond":24,
			"generate_audio":true,"service_tier":"default",
			"usage":{"completion_tokens":100,"total_tokens":100,"tool_usage":{"web_search":1}}
		}`),
	}
	task.Properties.OriginModelName = "endpoint-id"

	body, err := adaptor.ConvertToSeedanceVideo(task)
	require.NoError(t, err)
	var response taskResponse
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "endpoint-id", response.Model)
	assert.Equal(t, "succeeded", response.Status)
	assert.Nil(t, response.Error)
	assert.Equal(t, "https://cdn.example/video.mp4", response.Content.VideoURL)
	assert.Equal(t, "https://cdn.example/last.png", response.Content.LastFrameURL)
	require.NotNil(t, response.Usage)
	assert.Equal(t, 100, response.Usage.TotalTokens)
}

func TestConvertToOfficialTaskResponseHidesUpstreamFailure(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusFailure,
		Data: []byte(`{"status":"failed","error":{"code":"ContentFilter","message":"private upstream message"}}`),
	}
	body, err := (&TaskAdaptor{}).ConvertToSeedanceVideo(task)
	require.NoError(t, err)
	var response taskResponse
	require.NoError(t, common.Unmarshal(body, &response))
	require.NotNil(t, response.Error)
	assert.Equal(t, "ContentFilter", response.Error.Code)
	assert.Equal(t, publicFailureMessage, response.Error.Message)
	assert.NotContains(t, string(body), "private upstream message")
}

func TestParseOfficialTaskResult(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"upstream_task","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"},
		"usage":{"completion_tokens":120,"total_tokens":120}
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", result.Url)
	assert.Equal(t, 120, result.CompletionTokens)
}

func newTaskContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	return context, recorder
}
