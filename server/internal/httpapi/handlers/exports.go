package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"com.bestony.health-connect/internal/export"
	"com.bestony.health-connect/internal/httpapi/middleware"
	"com.bestony.health-connect/internal/store/sqlite"

	"github.com/gin-gonic/gin"
)

var exportIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Exports is the upload API used by the iOS app.
type Exports struct {
	Svc *export.Service
}

type createExportBody struct {
	StartedAt string `json:"startedAt"`
}

type completeExportBody struct {
	SampleCount int64 `json:"sampleCount"`
	BatchCount  int64 `json:"batchCount"`
}

// Create starts an export.
func (h Exports) Create(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body createExportBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body")
		return
	}
	if _, err := parseExportTime(body.StartedAt); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_body")
		return
	}
	id, err := h.Svc.Create(c.Request.Context(), userID, body.StartedAt)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"exportId": id})
}

// Batch stores one raw NDJSON body.
func (h Exports) Batch(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := c.Param("id")
	if !exportIDPattern.MatchString(id) {
		writeError(c, http.StatusNotFound, "not_found")
		return
	}
	err := h.Svc.AddBatch(c.Request.Context(), userID, id, c.Request.Body)
	if err == nil {
		c.Status(http.StatusNoContent)
		return
	}
	switch {
	case errors.Is(err, export.ErrEmpty):
		writeError(c, http.StatusBadRequest, "empty_body")
	case errors.Is(err, export.ErrTooLarge):
		writeError(c, http.StatusRequestEntityTooLarge, "upload_too_large")
	case errors.Is(err, sqlite.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found")
	case errors.Is(err, sqlite.ErrExportClosed):
		writeError(c, http.StatusConflict, "export_closed")
	default:
		writeError(c, http.StatusInternalServerError, "internal")
	}
}

// Complete stores the manifest and loads the batches.
func (h Exports) Complete(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := c.Param("id")
	if !exportIDPattern.MatchString(id) {
		writeError(c, http.StatusNotFound, "not_found")
		return
	}
	var body completeExportBody
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil || body.SampleCount < 0 || body.BatchCount < 0 {
		writeError(c, http.StatusBadRequest, "invalid_body")
		return
	}
	exp, err := h.Svc.Complete(c.Request.Context(), userID, id, body.SampleCount, body.BatchCount)
	if err != nil {
		switch {
		case errors.Is(err, sqlite.ErrNotFound):
			writeError(c, http.StatusNotFound, "not_found")
		default:
			writeError(c, http.StatusInternalServerError, "internal")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sampleCount":        exp.SampleCount,
		"batchCount":         exp.BatchCount,
		"receivedBatchCount": exp.ReceivedBatchCount,
		"completedAt":        exp.CompletedAt,
	})
}

func parseExportTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, raw)
}
