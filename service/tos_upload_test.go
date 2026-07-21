package service

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setTOSUploadTestEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("TOS_ACCESS_KEY", "test-access-key")
	t.Setenv("TOS_SECRET_KEY", "test-secret-key")
	t.Setenv("TOS_ENDPOINT", "https://tos-cn-beijing.volces.com")
	t.Setenv("TOS_REGION", "cn-beijing")
	t.Setenv("TOS_BUCKET", "test-bucket")
	t.Setenv("TOS_UPLOAD_PREFIX", "user-files")
	t.Setenv("TOS_UPLOAD_PRESIGN_EXPIRES", "600")
}

func TestGenerateTOSUploadPresign(t *testing.T) {
	setTOSUploadTestEnvironment(t)
	before := time.Now().Unix()

	result, err := GenerateTOSUploadPresign(42, &dto.UploadPresignRequest{
		Filename:    "../../avatar.PNG",
		ContentType: "image/png; charset=binary",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, http.MethodPut, result.Method)
	assert.True(t, strings.HasPrefix(result.ObjectKey, "user-files/42/"))
	assert.True(t, strings.HasSuffix(result.ObjectKey, ".png"))
	assert.NotContains(t, result.ObjectKey, "avatar")
	assert.Equal(t, "image/png", result.Headers["Content-Type"])
	assert.Equal(t, "https://test-bucket.tos-cn-beijing.volces.com/"+result.ObjectKey, result.PublicURL)
	assert.GreaterOrEqual(t, result.ExpiresAt, before+599)
	assert.LessOrEqual(t, result.ExpiresAt, time.Now().Unix()+601)

	parsedURL, err := url.Parse(result.URL)
	require.NoError(t, err)
	assert.Equal(t, "https", parsedURL.Scheme)
	assert.Contains(t, parsedURL.Host, "test-bucket")
	assert.Contains(t, parsedURL.EscapedPath(), result.ObjectKey)
	assert.NotEmpty(t, parsedURL.Query().Get("X-Tos-Signature"))
}

func TestGenerateTOSUploadPresignRejectsInvalidRequest(t *testing.T) {
	setTOSUploadTestEnvironment(t)

	_, err := GenerateTOSUploadPresign(42, &dto.UploadPresignRequest{
		Filename:    "avatar.png",
		ContentType: "invalid content type\r\nX-Test: value",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidUploadPresignRequest)
}

func TestGenerateTOSUploadPresignRequiresConfiguration(t *testing.T) {
	t.Setenv("TOS_ACCESS_KEY", "")
	t.Setenv("TOS_SECRET_KEY", "")
	t.Setenv("TOS_ENDPOINT", "")
	t.Setenv("TOS_REGION", "")
	t.Setenv("TOS_BUCKET", "")

	_, err := GenerateTOSUploadPresign(42, &dto.UploadPresignRequest{Filename: "file.bin"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTOSUploadConfiguration)
}
