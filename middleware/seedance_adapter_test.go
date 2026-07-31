package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceRequestConvertMapsOfficialContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", SeedanceRequestConvert(), func(c *gin.Context) {
		var body map[string]any
		require.NoError(t, common.UnmarshalBodyReusable(c, &body))
		c.JSON(http.StatusOK, body)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"S-MOYIN","content":[
		{"type":"text","text":"follow the references"},
		{"type":"image_url","image_url":{"url":"https://cdn.example/image.png"},"role":"reference_image"},
		{"type":"video_url","video_url":{"url":"https://cdn.example/video.mp4"},"role":"reference_video"},
		{"type":"audio_url","audio_url":{"url":"https://cdn.example/audio.mp3"},"role":"reference_audio"}],
		"duration":10,"ratio":"9:16","resolution":"720p"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "S-MOYIN", response["model"])
	assert.Equal(t, "follow the references", response["prompt"])
	assert.Equal(t, []any{"https://cdn.example/image.png"}, response["images"])
	assert.Equal(t, []any{"https://cdn.example/video.mp4"}, response["videos"])
	assert.Equal(t, []any{"https://cdn.example/audio.mp3"}, response["audios"])
	assert.Equal(t, "10", response["seconds"])
	assert.Equal(t, "9:16", response["aspect_ratio"])
	assert.Equal(t, "720p", response["resolution"])
	assert.Len(t, response["content"], 4)
}

func TestSeedanceRequestConvertAcceptsOfficialFieldsUnsupportedByMoyin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", SeedanceRequestConvert(), func(c *gin.Context) {
		var body map[string]any
		require.NoError(t, common.UnmarshalBodyReusable(c, &body))
		c.JSON(http.StatusOK, body)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"S-MOYIN",
		"content":[
			{"type":"text","text":"transition"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"},
			{"type":"draft_task","draft_task":{"id":"draft_task_123"}}
		],
		"callback_url":"https://client.example/callback",
		"return_last_frame":true,
		"service_tier":"default",
		"execution_expires_after":3600,
		"generate_audio":false,
		"draft":false,
		"tools":[{"type":"web_search"}],
		"safety_identifier":"user_hash",
		"priority":5,
		"frames":121,
		"seed":1,
		"camera_fixed":false,
		"watermark":true
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, true, response["return_last_frame"])
	assert.Equal(t, "https://client.example/callback", response["callback_url"])
	assert.Equal(t, []any{"https://cdn.example/first.png"}, response["images"])
	assert.Len(t, response["content"], 4)
}

func TestSeedanceRequestConvertAcceptsImageOnlyOfficialRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", SeedanceRequestConvert(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{
		"model":"endpoint-id",
		"content":[{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
}
