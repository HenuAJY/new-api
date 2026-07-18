package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondTaskErrorHidesUpstreamMessageAndPreservesStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	taskErr := &dto.TaskError{
		Code:          "fail_to_fetch_task",
		Message:       "<html>provider secret response</html>",
		StatusCode:    http.StatusBadRequest,
		UpstreamError: true,
		Error:         errors.New("<html>provider secret response</html>"),
	}

	respondTaskError(context, taskErr)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response dto.TaskError
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "fail_to_fetch_task", response.Code)
	assert.Equal(t, "video generation request failed", response.Message)
	assert.NotContains(t, recorder.Body.String(), "provider secret response")
}

func TestRespondTaskErrorPreservesLocalMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	taskErr := &dto.TaskError{
		Code:       "invalid_request",
		Message:    "model is required",
		StatusCode: http.StatusBadRequest,
		LocalError: true,
	}

	respondTaskError(context, taskErr)

	var response dto.TaskError
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "model is required", response.Message)
}
