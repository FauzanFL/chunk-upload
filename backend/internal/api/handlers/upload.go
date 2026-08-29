package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/chunked-upload/internal/logger"
	"github.com/yourorg/chunked-upload/internal/models"
	"github.com/yourorg/chunked-upload/internal/services/upload"
	"go.uber.org/zap"
)

// UploadHandler holds all upload-related HTTP handlers.
type UploadHandler struct {
	svc *upload.Service
}

// New returns an initialised UploadHandler.
func New(svc *upload.Service) *UploadHandler {
	return &UploadHandler{svc: svc}
}

// ─── Stage 1 – Init ──────────────────────────────────────────────────────────

// InitUpload godoc
// @Summary      Initialise a resumable upload session
// @Description  Client sends file metadata. Server returns an UploadID, ideal chunk size, and total chunk count.
// @Tags         upload
// @Accept       json
// @Produce      json
// @Param        body  body      models.InitUploadRequest   true  "File metadata"
// @Success      201   {object}  models.InitUploadResponse
// @Failure      400   {object}  models.ErrorResponse
// @Failure      413   {object}  models.ErrorResponse  "File too large"
// @Failure      415   {object}  models.ErrorResponse  "Unsupported MIME type"
// @Failure      429   {object}  models.ErrorResponse  "Rate limit exceeded"
// @Failure      500   {object}  models.ErrorResponse
// @Router       /upload/init [post]
func (h *UploadHandler) InitUpload(c *gin.Context) {
	var req models.InitUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid request body",
			Details: err.Error(),
		})
		return
	}

	resp, err := h.svc.Init(c.Request.Context(), &req)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case containsSubstr(err.Error(), "mime type"):
			status = http.StatusUnsupportedMediaType
		case containsSubstr(err.Error(), "exceeds maximum"):
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, models.ErrorResponse{Error: err.Error()})
		return
	}

	logger.Get().Info("upload initialised",
		zap.String("upload_id", resp.UploadID),
		zap.String("ip", c.ClientIP()),
		zap.String("file_name", req.FileName),
		zap.Int64("file_size", req.FileSize),
		zap.Int("total_chunks", resp.TotalChunks),
	)

	c.JSON(http.StatusCreated, resp)
}

// ─── Stage 2 – Upload Chunk ──────────────────────────────────────────────────

// UploadChunk godoc
// @Summary      Upload a single chunk
// @Description  Streams a raw binary chunk to object storage. upload_id and chunk_index are passed as query parameters. The request body is the raw chunk bytes.
// @Tags         upload
// @Accept       application/octet-stream
// @Produce      json
// @Param        upload_id    query     string  true  "Upload session ID"
// @Param        chunk_index  query     int     true  "Zero-based chunk index"
// @Param        body         body      string  true  "Raw chunk bytes"
// @Success      200          {object}  models.ChunkUploadResponse
// @Failure      400          {object}  models.ErrorResponse
// @Failure      404          {object}  models.ErrorResponse
// @Failure      429          {object}  models.ErrorResponse
// @Failure      500          {object}  models.ErrorResponse
// @Router       /upload/chunk [post]
func (h *UploadHandler) UploadChunk(c *gin.Context) {
	start := time.Now()
	log := logger.Get()

	uploadID := c.Query("upload_id")
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "upload_id is required"})
		return
	}

	chunkIndexStr := c.Query("chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "chunk_index must be a non-negative integer"})
		return
	}

	// The body IS the chunk – read it into memory (5MB is small enough)
	// so that it implements io.ReadSeeker for the AWS SDK payload hash.
	body := c.Request.Body
	defer body.Close()

	data, readErr := io.ReadAll(io.LimitReader(body, 6*1024*1024))
	if readErr != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "failed to read chunk body"})
		return
	}
	contentLength := int64(len(data))
	seekableBody := bytes.NewReader(data)

	resp, err := h.svc.UploadChunk(c.Request.Context(), uploadID, chunkIndex, contentLength, seekableBody)
	if err != nil {
		status := http.StatusInternalServerError
		if containsSubstr(err.Error(), "not found") || containsSubstr(err.Error(), "expired") {
			status = http.StatusNotFound
		} else if containsSubstr(err.Error(), "out of range") {
			status = http.StatusBadRequest
		}
		log.Error("chunk upload failed",
			zap.String("upload_id", uploadID),
			zap.Int("chunk_index", chunkIndex),
			zap.String("ip", c.ClientIP()),
			zap.Error(err),
		)
		c.JSON(status, models.ErrorResponse{Error: err.Error()})
		return
	}

	elapsed := time.Since(start)
	log.Info("chunk uploaded",
		zap.String("timestamp", time.Now().UTC().Format(time.RFC3339Nano)),
		zap.String("ip", c.ClientIP()),
		zap.String("upload_id", uploadID),
		zap.Int("chunk_index", chunkIndex),
		zap.Int64("chunk_size_bytes", contentLength),
		zap.Duration("duration", elapsed),
	)

	c.JSON(http.StatusOK, resp)
}

// ─── Stage 3 – Complete ──────────────────────────────────────────────────────

// CompleteUpload godoc
// @Summary      Finalise a resumable upload
// @Description  Validates all chunks are present, completes the S3 multipart upload, and removes the Redis session.
// @Tags         upload
// @Produce      json
// @Param        upload_id  query     string  true  "Upload session ID"
// @Success      200        {object}  models.CompleteUploadResponse
// @Failure      400        {object}  models.ErrorResponse
// @Failure      404        {object}  models.ErrorResponse
// @Failure      422        {object}  models.ErrorResponse  "Incomplete upload"
// @Failure      429        {object}  models.ErrorResponse
// @Failure      500        {object}  models.ErrorResponse
// @Router       /upload/complete [post]
func (h *UploadHandler) CompleteUpload(c *gin.Context) {
	uploadID := c.Query("upload_id")
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "upload_id is required"})
		return
	}

	resp, err := h.svc.Complete(c.Request.Context(), uploadID)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case containsSubstr(err.Error(), "not found") || containsSubstr(err.Error(), "expired"):
			status = http.StatusNotFound
		case containsSubstr(err.Error(), "missing chunks"):
			status = http.StatusUnprocessableEntity
		}
		c.JSON(status, models.ErrorResponse{Error: err.Error()})
		return
	}

	logger.Get().Info("upload completed",
		zap.String("upload_id", uploadID),
		zap.String("location", resp.Location),
		zap.String("ip", c.ClientIP()),
	)
	c.JSON(http.StatusOK, resp)
}

// ─── Status ──────────────────────────────────────────────────────────────────

// UploadStatus godoc
// @Summary      Get upload progress
// @Description  Returns the list of uploaded chunk indices so the client can resume without re-uploading completed chunks.
// @Tags         upload
// @Produce      json
// @Param        upload_id  query     string  true  "Upload session ID"
// @Success      200        {object}  models.UploadStatusResponse
// @Failure      400        {object}  models.ErrorResponse
// @Failure      404        {object}  models.ErrorResponse
// @Failure      429        {object}  models.ErrorResponse
// @Router       /upload/status [get]
func (h *UploadHandler) UploadStatus(c *gin.Context) {
	uploadID := c.Query("upload_id")
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "upload_id is required"})
		return
	}

	resp, err := h.svc.Status(c.Request.Context(), uploadID)
	if err != nil {
		status := http.StatusInternalServerError
		if containsSubstr(err.Error(), "not found") || containsSubstr(err.Error(), "expired") {
			status = http.StatusNotFound
		}
		c.JSON(status, models.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ─── Health ──────────────────────────────────────────────────────────────────

// Health godoc
// @Summary      Health check
// @Description  Returns 200 OK when the service is running.
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /health [get]
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "chunked-upload"})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// bytesReader turns a []byte into an io.ReadCloser without importing bytes package at top level.
type bytesReadCloser struct{ r io.Reader }

func (b *bytesReadCloser) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *bytesReadCloser) Close() error               { return nil }

func bytesReader(data []byte) io.ReadCloser {
	return &bytesReadCloser{r: readerFromBytes(data)}
}

type simpleReader struct {
	data []byte
	pos  int
}

func readerFromBytes(data []byte) io.Reader {
	return &simpleReader{data: data}
}

func (r *simpleReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// maxFileSizeMiddleware rejects requests whose Content-Length exceeds the limit.
func MaxChunkSizeMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, models.ErrorResponse{
				Error:   "chunk too large",
				Details: fmt.Sprintf("maximum chunk size is %d bytes", maxBytes),
			})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
