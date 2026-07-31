package gravitexsd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestBuildRequestPreservesSupportedOfficialFields(t *testing.T) {
	assetServer := newAssetTestServer(t)
	defer assetServer.Close()
	context := testContext(`{
		"model":"downstream-seedance","prompt":"projected prompt","duration":15,
		"resolution":"1080p","ratio":"adaptive","generate_audio":false,
		"watermark":true,"seed":0,
		"content":[
			{"type":"text","text":"follow the references"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"},
			{"type":"audio_url","audio_url":{"url":"https://cdn.example/audio.mp3"},"role":"reference_audio"},
			{"type":"draft_task","draft_task":{"id":"draft-unsupported"}}
		],
		"callback_url":"https://client.example/callback","return_last_frame":true
	}`)
	info := testRelayInfo()
	info.ChannelBaseUrl = assetServer.URL
	info.ApiKey = "sk-asset-test"
	info.OriginModelName = "downstream-seedance"
	info.UpstreamModelName = "provider-custom-model"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	bodyReader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"provider-custom-model","duration":15,"resolution":"1080p","ratio":"adaptive",
		"generate_audio":false,"watermark":true,"seed":0,
		"content":[
			{"type":"text","text":"follow the references"},
			{"type":"image_url","image_url":{"url":"asset://asset-1"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"asset://asset-2"},"role":"last_frame"},
			{"type":"audio_url","audio_url":{"url":"asset://asset-3"},"role":"reference_audio"}
		]
	}`, string(body))
	assert.NotContains(t, string(body), "draft_task")
	assert.NotContains(t, string(body), "callback_url")
	assert.NotContains(t, string(body), "return_last_frame")
	assert.Equal(t, map[string]float64{"seconds": 15}, adaptor.EstimateBilling(context, info))
}

func newAssetTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	assetNumber := 0
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/asset-groups":
			_, _ = writer.Write([]byte(`{"groups":[]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/asset-groups":
			_, _ = writer.Write([]byte(`{"group_id":"group-test"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/assets":
			var payload map[string]string
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.Equal(t, "group-test", payload["group_id"])
			assert.Contains(t, []string{"Image", "Video", "Audio"}, payload["asset_type"])
			assetNumber++
			_, _ = writer.Write([]byte(`{"virtual_id":"asset-` + strconv.Itoa(assetNumber) + `","status":"pending"}`))
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/assets/"):
			virtualID := strings.TrimPrefix(request.URL.Path, "/v1/assets/")
			_, _ = writer.Write([]byte(`{"virtual_id":"` + virtualID + `","asset_url":"asset://` + virtualID + `","status":"active"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
}

func TestBuildRequestSupportsPromptAutoDurationAndArbitraryMappedModel(t *testing.T) {
	context := testContext(`{
		"model":"arbitrary-downstream-model","prompt":"generate","duration":-1,
		"resolution":"480p","ratio":"21:9"
	}`)
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
	assert.JSONEq(t, `{
		"model":"arbitrary-provider-model","prompt":"generate","duration":-1,
		"resolution":"480p","ratio":"21:9"
	}`, string(body))
	assert.Equal(t, []string{"seedance-2-0", "seedance-2-0-fast"}, adaptor.GetModelList())
	assert.Equal(t, map[string]float64{"seconds": 5}, adaptor.EstimateBilling(context, info))

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.gravitex.ai/v1/video/generations", requestURL)
}

func TestEstimateBillingUsesDefaultForAutomaticDuration(t *testing.T) {
	context := testContext(`{"model":"any-downstream-model","prompt":"generate","duration":-1}`)
	info := testRelayInfo()
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	assert.Equal(t, map[string]float64{"seconds": 5}, adaptor.EstimateBilling(context, info))
}

func TestValidateRequestEnforcesGravitexRules(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"duration too short", `{"model":"any","prompt":"generate","duration":3}`},
		{"duration too long", `{"model":"any","prompt":"generate","duration":16}`},
		{"multiple first frames", `{
			"model":"any","content":[{"type":"text","text":"generate"},
			{"type":"image_url","image_url":{"url":"1"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"2"},"role":"first_frame"}]}`},
		{"first frame conflicts with reference", `{
			"model":"any","content":[{"type":"text","text":"generate"},
			{"type":"image_url","image_url":{"url":"1"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"2"},"role":"reference_image"}]}`},
		{"last frame requires first", `{
			"model":"any","content":[{"type":"text","text":"generate"},
			{"type":"image_url","image_url":{"url":"2"},"role":"last_frame"}]}`},
		{"audio requires visual reference", `{
			"model":"any","content":[{"type":"text","text":"generate"},
			{"type":"audio_url","audio_url":{"url":"audio"},"role":"reference_audio"}]}`},
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

func TestDoResponseReturnsPublicTaskIDAndHidesProviderMessage(t *testing.T) {
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

	failureResponse := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body: io.NopCloser(strings.NewReader(`{
			"error":{"code":"InvalidParameter","message":"private provider details"}
		}`)),
	}
	_, _, taskErr = (&TaskAdaptor{}).DoResponse(context, failureResponse, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "InvalidParameter", taskErr.Code)
	assert.Equal(t, publicFailureMessage, taskErr.Message)
}

func TestParseTaskResultRequiresVideoURLForCompletion(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"status":"completed","progress":100,"video_url":"https://cdn.example/result.mp4"
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
		"status":"completed","metadata":{"video_url":"https://cdn.example/fallback.mp4"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/fallback.mp4", result.Url)
}

func TestParseTaskResultUnwrapsGravitexDataEnvelope(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"code":"success","message":"","data":{
			"id":"cgt-task","status":"completed","progress":100,
			"created_at":1785077777,"completed_at":1785077998,
			"metadata":{"content":{"video_url":"https://cdn.example/enveloped.mp4"}},
			"seconds":"4"
		}
	}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, taskcommon.ProgressComplete, result.Progress)
	assert.Equal(t, "https://cdn.example/enveloped.mp4", result.Url)
	assert.Equal(t, int64(1785077777), result.StartTime)
	assert.Equal(t, int64(1785077998), result.FinishTime)
}

func TestParseInProgressTaskClampsCompleteProgress(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"in_progress","progress":100}`))
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
	successTask.Properties.OriginModelName = "downstream-seedance"
	body, err := adaptor.ConvertToSeedanceVideo(successTask)
	require.NoError(t, err)
	var success dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &success))
	assert.Equal(t, "succeeded", success.Status)
	assert.Equal(t, "https://cdn.example/result.mp4", success.Content.VideoURL)

	failureTask := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusFailure,
		FailReason: "private poll failure", Data: []byte(`{
			"status":"failed","error":{"code":"failed","message":"private upstream message"}
		}`),
	}
	failureTask.Properties.OriginModelName = "downstream-seedance"
	body, err = adaptor.ConvertToSeedanceVideo(failureTask)
	require.NoError(t, err)
	var failure dto.SeedanceTaskResponse
	require.NoError(t, common.Unmarshal(body, &failure))
	require.NotNil(t, failure.Error)
	assert.Equal(t, "failed", failure.Error.Code)
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
