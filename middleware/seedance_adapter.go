package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

const seedanceAPIContextKey = "seedance_api"

func SeedanceRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set(seedanceAPIContextKey, true)
		if c.Request.Method == http.MethodGet {
			c.Request.URL.Path = "/v1/video/generations/" + c.Param("task_id")
			c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
			c.Next()
			return
		}

		var request dto.SeedanceTaskRequest
		if err := common.UnmarshalBodyReusable(c, &request); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "invalid_request", Message: err.Error()})
			return
		}
		if strings.TrimSpace(request.Model) == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "missing_model", Message: "model is required"})
			return
		}

		prompts := make([]string, 0, 1)
		images := make([]string, 0)
		videos := make([]string, 0)
		audios := make([]string, 0)
		hasDraftTask := false
		for _, item := range request.Content {
			switch item.Type {
			case "text":
				if text := strings.TrimSpace(item.Text); text != "" {
					prompts = append(prompts, text)
				}
			case "image_url":
				if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
					c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "invalid_request", Message: "image_url.url is required"})
					return
				}
				if item.Role == "last_frame" {
					continue
				}
				images = append(images, item.ImageURL.URL)
			case "video_url":
				if item.VideoURL == nil || strings.TrimSpace(item.VideoURL.URL) == "" {
					c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "invalid_request", Message: "video_url.url is required"})
					return
				}
				videos = append(videos, item.VideoURL.URL)
			case "audio_url":
				if item.AudioURL == nil || strings.TrimSpace(item.AudioURL.URL) == "" {
					c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "invalid_request", Message: "audio_url.url is required"})
					return
				}
				audios = append(audios, item.AudioURL.URL)
			case "draft_task":
				hasDraftTask = item.DraftTask != nil && strings.TrimSpace(item.DraftTask.ID) != ""
			default:
				c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "unsupported_content_type", Message: fmt.Sprintf("content type %s is not supported", item.Type)})
				return
			}
		}

		prompt := strings.Join(prompts, "\n")
		if prompt == "" && len(images) == 0 && len(videos) == 0 && !hasDraftTask {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "invalid_request", Message: "content must include text, image, video, or draft_task"})
			return
		}
		if len(audios) > 0 && len(images) == 0 && len(videos) == 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "invalid_request", Message: "audio reference requires an image or video reference"})
			return
		}

		// Keep the official request intact so each channel adaptor can consume
		// the fields it supports, then add the common provider projections.
		var upstreamRequest map[string]any
		if err := common.UnmarshalBodyReusable(c, &upstreamRequest); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.TaskError{Code: "invalid_request", Message: err.Error()})
			return
		}
		upstreamRequest["model"] = request.Model
		upstreamRequest["prompt"] = prompt
		upstreamRequest["images"] = images
		upstreamRequest["videos"] = videos
		upstreamRequest["audios"] = audios
		upstreamRequest["aspect_ratio"] = request.Ratio
		upstreamRequest["resolution"] = request.Resolution
		if request.Duration != nil {
			upstreamRequest["seconds"] = fmt.Sprintf("%d", *request.Duration)
		}
		body, err := common.Marshal(upstreamRequest)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, dto.TaskError{Code: "marshal_request_failed", Message: err.Error()})
			return
		}
		common.CleanupBodyStorage(c)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.URL.Path = "/v1/video/generations"
		c.Set(common.KeyRequestBody, body)
		c.Set("relay_mode", relayconstant.RelayModeVideoSubmit)
		c.Next()
	}
}
