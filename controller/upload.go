package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func CreateUploadPresign(c *gin.Context) {
	request := &dto.UploadPresignRequest{}
	if err := common.DecodeJson(c.Request.Body, request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": types.OpenAIError{
				Message: "invalid request body",
				Type:    "invalid_request_error",
				Param:   "",
				Code:    "invalid_request_body",
			},
		})
		return
	}

	response, err := service.GenerateTOSUploadPresign(c.GetInt("id"), request)
	if err == nil {
		c.JSON(http.StatusOK, response)
		return
	}

	if errors.Is(err, service.ErrInvalidUploadPresignRequest) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": types.OpenAIError{
				Message: err.Error(),
				Type:    "invalid_request_error",
				Param:   "",
				Code:    "invalid_upload_request",
			},
		})
		return
	}
	if errors.Is(err, service.ErrTOSUploadConfiguration) {
		common.SysLog("TOS upload configuration error: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": types.OpenAIError{
				Message: "upload service is not configured",
				Type:    "new_api_error",
				Param:   "",
				Code:    "upload_service_unavailable",
			},
		})
		return
	}

	common.SysLog("failed to generate TOS upload pre-signed URL: " + err.Error())
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": types.OpenAIError{
			Message: "failed to generate upload URL",
			Type:    "new_api_error",
			Param:   "",
			Code:    "upload_presign_failed",
		},
	})
}
