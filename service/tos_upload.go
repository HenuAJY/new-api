package service

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/dto"

	"github.com/google/uuid"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

const (
	defaultTOSUploadPrefix        = "uploads"
	defaultTOSUploadPresignExpiry = 900
	maxTOSUploadPresignExpiry     = 7 * 24 * 60 * 60
)

var (
	ErrInvalidUploadPresignRequest = errors.New("invalid upload presign request")
	ErrTOSUploadConfiguration      = errors.New("invalid TOS upload configuration")
)

func GenerateTOSUploadPresign(userID int, request *dto.UploadPresignRequest) (*dto.UploadPresignResponse, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("%w: invalid user", ErrInvalidUploadPresignRequest)
	}
	if request == nil {
		return nil, fmt.Errorf("%w: request body is required", ErrInvalidUploadPresignRequest)
	}

	filename := strings.TrimSpace(request.Filename)
	if filename == "" {
		return nil, fmt.Errorf("%w: filename is required", ErrInvalidUploadPresignRequest)
	}
	if len(filename) > 255 {
		return nil, fmt.Errorf("%w: filename must not exceed 255 bytes", ErrInvalidUploadPresignRequest)
	}

	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	} else {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid content_type", ErrInvalidUploadPresignRequest)
		}
		contentType = mediaType
	}

	accessKey := os.Getenv("TOS_ACCESS_KEY")
	secretKey := os.Getenv("TOS_SECRET_KEY")
	endpoint := strings.TrimSpace(os.Getenv("TOS_ENDPOINT"))
	region := strings.TrimSpace(os.Getenv("TOS_REGION"))
	bucket := strings.TrimSpace(os.Getenv("TOS_BUCKET"))
	if accessKey == "" || secretKey == "" || endpoint == "" || region == "" || bucket == "" {
		return nil, fmt.Errorf("%w: TOS_ACCESS_KEY, TOS_SECRET_KEY, TOS_ENDPOINT, TOS_REGION and TOS_BUCKET are required", ErrTOSUploadConfiguration)
	}

	expires := int64(defaultTOSUploadPresignExpiry)
	if value := strings.TrimSpace(os.Getenv("TOS_UPLOAD_PRESIGN_EXPIRES")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 || parsed > maxTOSUploadPresignExpiry {
			return nil, fmt.Errorf("%w: TOS_UPLOAD_PRESIGN_EXPIRES must be between 1 and %d", ErrTOSUploadConfiguration, maxTOSUploadPresignExpiry)
		}
		expires = parsed
	}

	prefix := strings.Trim(strings.TrimSpace(os.Getenv("TOS_UPLOAD_PREFIX")), "/")
	if prefix == "" {
		prefix = defaultTOSUploadPrefix
	}
	cleanPrefix := path.Clean(prefix)
	if cleanPrefix == "." || cleanPrefix == ".." || strings.HasPrefix(cleanPrefix, "../") {
		return nil, fmt.Errorf("%w: TOS_UPLOAD_PREFIX must be a valid object key prefix", ErrTOSUploadConfiguration)
	}

	baseName := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	extension := strings.ToLower(path.Ext(baseName))
	if len(extension) > 16 {
		extension = ""
	}
	for _, char := range extension {
		if char != '.' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			extension = ""
			break
		}
	}

	now := time.Now().UTC()
	objectKey := path.Join(
		cleanPrefix,
		strconv.Itoa(userID),
		now.Format("2006/01/02"),
		uuid.NewString()+extension,
	)
	headers := map[string]string{"Content-Type": contentType}

	client, err := tos.NewClientV2(
		endpoint,
		tos.WithRegion(region),
		tos.WithCredentials(tos.NewStaticCredentials(accessKey, secretKey)),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTOSUploadConfiguration, err)
	}

	signed, err := client.PreSignedURL(&tos.PreSignedURLInput{
		HTTPMethod:         http.MethodPut,
		Bucket:             bucket,
		Key:                objectKey,
		Expires:            expires,
		Header:             headers,
		IsSignedAllHeaders: true,
	})
	if err != nil {
		return nil, fmt.Errorf("generate TOS upload pre-signed URL: %w", err)
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil || endpointURL.Scheme == "" || endpointURL.Host == "" {
		return nil, fmt.Errorf("%w: TOS_ENDPOINT must be an absolute URL", ErrTOSUploadConfiguration)
	}
	if endpointURL.Scheme != "http" && endpointURL.Scheme != "https" {
		return nil, fmt.Errorf("%w: TOS_ENDPOINT must use HTTP or HTTPS", ErrTOSUploadConfiguration)
	}
	if strings.Trim(endpointURL.Path, "/") != "" || endpointURL.RawQuery != "" || endpointURL.Fragment != "" {
		return nil, fmt.Errorf("%w: TOS_ENDPOINT must not contain a path, query or fragment", ErrTOSUploadConfiguration)
	}
	endpointURL.Host = bucket + "." + endpointURL.Host
	endpointURL.Path = "/" + objectKey

	return &dto.UploadPresignResponse{
		URL:       signed.SignedUrl,
		PublicURL: endpointURL.String(),
		Method:    http.MethodPut,
		ObjectKey: objectKey,
		Headers:   signed.SignedHeader,
		ExpiresAt: now.Add(time.Duration(expires) * time.Second).Unix(),
	}, nil
}
