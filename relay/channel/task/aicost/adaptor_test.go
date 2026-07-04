package aicost

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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

func TestAicostBuildsGrokImagineVideo15Request(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"grok-imagine-video-1.5-preview",
		"prompt":"walk forward",
		"input_reference":{"image_url":"data:image/jpeg;base64,first"},
		"seconds":"10",
		"size":"1280x720"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://www.aicost.xyz", ApiKey: "sk-test"},
	})
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	info.UpstreamModelName = "grok-imagine-video-1.5-preview"

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(bodyBytes, &payload))
	assert.Equal(t, "grok-imagine-video-1.5-preview", payload["model"])
	assert.Equal(t, "walk forward", payload["prompt"])
	assert.Equal(t, "10", payload["seconds"])
	assert.Equal(t, "1280x720", payload["size"])
	assert.Equal(t, []any{"data:image/jpeg;base64,first"}, payload["images"])
}

func TestAicostBuildsGrokImagineVideo15RequestWithMultipartInputReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	require.NoError(t, writer.WriteField("model", "grok-imagine-video-1.5-preview"))
	require.NoError(t, writer.WriteField("prompt", "walk forward"))
	require.NoError(t, writer.WriteField("seconds", "10"))
	require.NoError(t, writer.WriteField("size", "1280x720"))
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="input_reference"; filename="scene.png"`)
	header.Set("Content-Type", "image/png")
	filePart, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = filePart.Write([]byte("fake-png"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/videos", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	info.UpstreamModelName = "grok-imagine-video-1.5-preview"

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(bodyBytes, &payload))
	assert.Equal(t, "grok-imagine-video-1.5-preview", payload["model"])
	assert.Equal(t, "walk forward", payload["prompt"])
	assert.Equal(t, "10", payload["seconds"])
	assert.Equal(t, "1280x720", payload["size"])
	assert.Equal(t, []any{"data:image/png;base64,ZmFrZS1wbmc="}, payload["images"])
}

func TestAicostRejectsGrokImagineVideo15WithoutReferenceImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"grok-imagine-video-1.5-preview",
		"prompt":"walk forward",
		"seconds":"10"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(context, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}})

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "missing_reference_image", taskErr.Code)
}

func TestAicostDoResponseReturnsPublicOpenAIVideoForAsyncTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"task_id":"upstream_task","status":"pending"}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video-1.5-preview",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(context, response, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_task", upstreamTaskID)
	assert.JSONEq(t, `{"task_id":"upstream_task","status":"pending"}`, string(taskData))
	assert.Equal(t, http.StatusOK, recorder.Code)
	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &video))
	assert.Equal(t, "task_public", video.ID)
	assert.Equal(t, "task_public", video.TaskID)
	assert.Equal(t, "grok-imagine-video-1.5-preview", video.Model)
	assert.Equal(t, dto.VideoStatusQueued, video.Status)
}

func TestAicostDoResponseUnwrapsDataEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"data":{"task_id":"upstream_task","status":"completed","video_url":"https://cdn.example/video.mp4"}}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video-1.5-preview",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(context, response, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_task", upstreamTaskID)
	assert.JSONEq(t, `{"data":{"task_id":"upstream_task","status":"completed","video_url":"https://cdn.example/video.mp4"}}`, string(taskData))
	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &video))
	assert.Equal(t, dto.VideoStatusCompleted, video.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", video.Metadata["url"])
}

func TestAicostParseTaskResultAndConvertToOpenAIVideo(t *testing.T) {
	adaptor := &TaskAdaptor{}
	taskInfo, err := adaptor.ParseTaskResult([]byte(`{"task_id":"upstream_task","status":"completed","video_url":"https://cdn.example/video.mp4"}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", taskInfo.Url)

	originTask := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: 100,
		UpdatedAt: 200,
		Data:      []byte(`{"task_id":"upstream_task","status":"completed","video_url":"https://cdn.example/video.mp4"}`),
	}
	originTask.Properties.OriginModelName = "grok-imagine-video-1.5-preview"

	videoBytes, err := adaptor.ConvertToOpenAIVideo(originTask)
	require.NoError(t, err)

	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(videoBytes, &video))
	assert.Equal(t, "task_public", video.ID)
	assert.Equal(t, dto.VideoStatusCompleted, video.Status)
	assert.Equal(t, "grok-imagine-video-1.5-preview", video.Model)
	assert.Equal(t, "https://cdn.example/video.mp4", video.Metadata["url"])
}

func TestAicostParseTaskResultAndConvertToOpenAIVideoUnwrapDataEnvelope(t *testing.T) {
	adaptor := &TaskAdaptor{}
	envelopedBody := []byte(`{"data":{"task_id":"upstream_task","status":"completed","video_url":"https://cdn.example/video.mp4"}}`)
	taskInfo, err := adaptor.ParseTaskResult(envelopedBody)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", taskInfo.Url)

	originTask := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: 100,
		UpdatedAt: 200,
		Data:      envelopedBody,
	}
	originTask.Properties.OriginModelName = "grok-imagine-video-1.5-preview"

	videoBytes, err := adaptor.ConvertToOpenAIVideo(originTask)
	require.NoError(t, err)

	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(videoBytes, &video))
	assert.Equal(t, "task_public", video.ID)
	assert.Equal(t, dto.VideoStatusCompleted, video.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", video.Metadata["url"])
}

func TestAicostParseTaskResultPrefersDataEnvelopeWhenTopLevelOmitsVideoURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{
		"id":"task_public",
		"task_id":"task_public",
		"status":"SUCCESS",
		"data":{
			"id":"task_public",
			"task_id":"task_public",
			"status":"completed",
			"video_url":"https://cdn.example/video.mp4"
		}
	}`)

	taskInfo, err := adaptor.ParseTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", taskInfo.Url)

	originTask := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: 100,
		UpdatedAt: 200,
		Data:      body,
	}
	originTask.Properties.OriginModelName = "grok-imagine-video-1.5-preview"

	videoBytes, err := adaptor.ConvertToOpenAIVideo(originTask)
	require.NoError(t, err)

	var video dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(videoBytes, &video))
	assert.Equal(t, "https://cdn.example/video.mp4", video.Metadata["url"])
}

func TestAicostBuildRequestURLAndHeaders(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://www.aicost.xyz", ApiKey: "sk-test"},
	})

	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{})
	require.NoError(t, err)
	assert.Equal(t, "https://www.aicost.xyz/v1/videos", url)

	request := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	require.NoError(t, adaptor.BuildRequestHeader(nil, request, &relaycommon.RelayInfo{}))
	assert.Equal(t, "Bearer sk-test", request.Header.Get("Authorization"))
	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
}

func TestAicostModelListContainsOnlyGrokImagineVideo15Preview(t *testing.T) {
	assert.Equal(t, []string{"grok-imagine-video-1.5-preview"}, (&TaskAdaptor{}).GetModelList())
	assert.Equal(t, "aicost", (&TaskAdaptor{}).GetChannelName())
	assert.Contains(t, ModelList, "grok-imagine-video-1.5-preview")
	assert.Equal(t, constant.ChannelTypeAicost, constant.ChannelTypeAicost)
}
