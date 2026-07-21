package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUploadPresign(t *testing.T) {
	t.Setenv("TOS_ACCESS_KEY", "test-access-key")
	t.Setenv("TOS_SECRET_KEY", "test-secret-key")
	t.Setenv("TOS_ENDPOINT", "https://tos-cn-beijing.volces.com")
	t.Setenv("TOS_REGION", "cn-beijing")
	t.Setenv("TOS_BUCKET", "test-bucket")
	t.Setenv("TOS_UPLOAD_PREFIX", "uploads")
	t.Setenv("TOS_UPLOAD_PRESIGN_EXPIRES", "900")

	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/upload/presign",
		strings.NewReader(`{"filename":"photo.png","content_type":"image/png"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Set("id", 123)

	CreateUploadPresign(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response dto.UploadPresignResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, http.MethodPut, response.Method)
	assert.True(t, strings.HasPrefix(response.ObjectKey, "uploads/123/"))
	assert.Equal(t, "image/png", response.Headers["Content-Type"])
	assert.NotEmpty(t, response.URL)
	assert.Equal(t, "https://test-bucket.tos-cn-beijing.volces.com/"+response.ObjectKey, response.PublicURL)
	assert.NotZero(t, response.ExpiresAt)
}
