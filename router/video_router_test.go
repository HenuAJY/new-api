package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestVideoContentPreflightReturnsCorsHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)

	request := httptest.NewRequest(http.MethodOptions, "/v1/videos/task_123/content", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	request.Header.Set("Access-Control-Request-Headers", "authorization")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.NotEmpty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, recorder.Header().Values("Access-Control-Allow-Headers"))
}
