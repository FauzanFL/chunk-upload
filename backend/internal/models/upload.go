package models

import "time"

// UploadStatus represents the lifecycle state of a multipart upload.
type UploadStatus string

const (
	StatusInitialized UploadStatus = "initialized"
	StatusInProgress  UploadStatus = "in_progress"
	StatusCompleted   UploadStatus = "completed"
	StatusFailed      UploadStatus = "failed"
)

// UploadMeta is the canonical record stored in Redis for every active upload.
type UploadMeta struct {
	UploadID        string       `json:"upload_id"`
	FileName        string       `json:"file_name"`
	FileSize        int64        `json:"file_size"`
	MimeType        string       `json:"mime_type"`
	MD5Hash         string       `json:"md5_hash"`
	SHA256Hash      string       `json:"sha256_hash"`
	ChunkSize       int64        `json:"chunk_size"`
	TotalChunks     int          `json:"total_chunks"`
	UploadedChunks  []int        `json:"uploaded_chunks"` // sorted list of completed chunk indices
	MinioUploadID   string       `json:"minio_upload_id"` // AWS multipart upload ID
	Status          UploadStatus `json:"status"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// ─── Request / Response DTOs ─────────────────────────────────────────────────

// InitUploadRequest is sent by the client to start a new upload session.
type InitUploadRequest struct {
	FileName   string `json:"file_name"   binding:"required"`
	FileSize   int64  `json:"file_size"   binding:"required,min=1"`
	MimeType   string `json:"mime_type"   binding:"required"`
	MD5Hash    string `json:"md5_hash"`
	SHA256Hash string `json:"sha256_hash"`
}

// InitUploadResponse is returned after successful initialisation.
type InitUploadResponse struct {
	UploadID    string `json:"upload_id"`
	ChunkSize   int64  `json:"chunk_size"`
	TotalChunks int    `json:"total_chunks"`
}

// ChunkUploadResponse is returned after a successful chunk upload.
type ChunkUploadResponse struct {
	UploadID       string `json:"upload_id"`
	ChunkIndex     int    `json:"chunk_index"`
	UploadedChunks int    `json:"uploaded_chunks"`
	TotalChunks    int    `json:"total_chunks"`
}

// UploadStatusResponse is returned by the status endpoint.
type UploadStatusResponse struct {
	UploadID        string       `json:"upload_id"`
	Status          UploadStatus `json:"status"`
	FileName        string       `json:"file_name"`
	FileSize        int64        `json:"file_size"`
	TotalChunks     int          `json:"total_chunks"`
	UploadedChunks  []int        `json:"uploaded_chunks"`
	NextChunkIndex  int          `json:"next_chunk_index"` // convenience field for client
	ProgressPercent float64      `json:"progress_percent"`
}

// CompleteUploadResponse is returned after a successful merge.
type CompleteUploadResponse struct {
	UploadID string `json:"upload_id"`
	FileName string `json:"file_name"`
	Location string `json:"location"` // final object key in MinIO
	Message  string `json:"message"`
}

// ErrorResponse wraps API error messages.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}
