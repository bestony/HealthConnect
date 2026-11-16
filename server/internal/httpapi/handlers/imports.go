package handlers

import (
	"errors"
	"net/http"

	"com.bestony.health-connect/internal/httpapi/middleware"
	"com.bestony.health-connect/internal/ingest"
	"com.bestony.health-connect/internal/store/sqlite"

	"github.com/gin-gonic/gin"
)

// Imports is the upload API.
type Imports struct {
	Svc       *ingest.Service
	MaxUpload int64
}

type importJob struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	Phase       string `json:"phase"`
	ByteSize    int64  `json:"byte_size"`
	RowsRead    int64  `json:"rows_read"`
	RowsWritten int64  `json:"rows_written"`
	Error       string `json:"error,omitempty"`
	CreatedAt   string `json:"created_at"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
}

func toJob(imp sqlite.Import) importJob {
	return importJob{
		ID: imp.ID, Status: imp.Status, Mode: imp.Mode, Phase: imp.Phase,
		ByteSize: imp.ByteSize, RowsRead: imp.RowsRead, RowsWritten: imp.RowsWritten,
		Error: imp.Error, CreatedAt: imp.CreatedAt, StartedAt: imp.StartedAt, FinishedAt: imp.FinishedAt,
	}
}

// Create accepts an Apple Health export.
func (h Imports) Create(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.MaxUpload)
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		if isTooLarge(err) {
			writeError(c, http.StatusRequestEntityTooLarge, "upload_too_large")
			return
		}
		writeError(c, http.StatusBadRequest, "invalid_body")
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, "file_required")
		return
	}
	defer file.Close()
	filename := ""
	if header != nil {
		filename = header.Filename
	}
	imp, err := h.Svc.Accept(c.Request.Context(), userID, filename, c.Request.FormValue("mode"), c.GetHeader("Idempotency-Key"), file)
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, toJob(imp))
}

// List returns recent jobs.
func (h Imports) List(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := h.Svc.List(c.Request.Context(), userID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal")
		return
	}
	jobs := make([]importJob, 0, len(items))
	for _, item := range items {
		jobs = append(jobs, toJob(item))
	}
	c.JSON(http.StatusOK, gin.H{"imports": jobs})
}

// Get returns one job.
func (h Imports) Get(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	imp, err := h.Svc.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, toJob(imp))
}

// Retry requeues a failed job.
func (h Imports) Retry(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	imp, err := h.Svc.Retry(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, toJob(imp))
}

func writeImportError(c *gin.Context, err error) {
	var coded *ingest.Error
	if errors.As(err, &coded) {
		writeError(c, http.StatusBadRequest, coded.Code)
		return
	}
	switch {
	case errors.Is(err, sqlite.ErrNotFound), errors.Is(err, ingest.ErrNoSpool):
		writeError(c, http.StatusNotFound, "not_found")
	case errors.Is(err, sqlite.ErrImportActive):
		writeError(c, http.StatusConflict, "import_active")
	case errors.Is(err, sqlite.ErrIdempotencyConflict):
		writeError(c, http.StatusConflict, "idempotency_conflict")
	case errors.Is(err, sqlite.ErrNotRetriable):
		writeError(c, http.StatusConflict, "not_retriable")
	default:
		if isTooLarge(err) {
			writeError(c, http.StatusRequestEntityTooLarge, "upload_too_large")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal")
	}
}

func isTooLarge(err error) bool {
	var max *http.MaxBytesError
	return errors.As(err, &max)
}

func writeError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code})
}
