package upload

import (
	"context"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"sort"
	"time"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"github.com/yourorg/chunked-upload/internal/config"
	"github.com/yourorg/chunked-upload/internal/models"
	"github.com/yourorg/chunked-upload/internal/services/cache"
	"github.com/yourorg/chunked-upload/internal/services/storage"
)

// Service orchestrates the three-stage chunked upload lifecycle.
type Service struct {
	cache   *cache.Service
	storage *storage.Service
	cfg     *config.Config
}

// New returns a ready-to-use upload service.
func New(cache *cache.Service, storage *storage.Service, cfg *config.Config) *Service {
	return &Service{cache: cache, storage: storage, cfg: cfg}
}

// ─── Stage 1 – Initialise ────────────────────────────────────────────────────

// Init validates the request, generates an UploadID, opens a MinIO multipart
// upload, persists metadata to Redis, and returns the session token and chunk size.
func (s *Service) Init(ctx context.Context, req *models.InitUploadRequest) (*models.InitUploadResponse, error) {
	// Validate MIME type
	if !s.isMimeAllowed(req.MimeType) {
		return nil, fmt.Errorf("mime type %q is not allowed", req.MimeType)
	}

	// Validate file size
	if req.FileSize > s.cfg.Upload.MaxFileSizeBytes {
		return nil, fmt.Errorf("file size %d exceeds maximum allowed %d bytes",
			req.FileSize, s.cfg.Upload.MaxFileSizeBytes)
	}

	chunkSize := s.cfg.Upload.ChunkSizeBytes
	totalChunks := int(math.Ceil(float64(req.FileSize) / float64(chunkSize)))
	if totalChunks < 1 {
		totalChunks = 1
	}

	uploadID := uuid.New().String()
	objectKey := objectKeyFor(uploadID, req.FileName)

	minioUploadID, err := s.storage.CreateMultipartUpload(ctx, objectKey, req.MimeType)
	if err != nil {
		return nil, fmt.Errorf("init multipart: %w", err)
	}

	meta := &models.UploadMeta{
		UploadID:       uploadID,
		FileName:       req.FileName,
		FileSize:       req.FileSize,
		MimeType:       req.MimeType,
		MD5Hash:        req.MD5Hash,
		SHA256Hash:     req.SHA256Hash,
		ChunkSize:      chunkSize,
		TotalChunks:    totalChunks,
		UploadedChunks: []int{},
		MinioUploadID:  minioUploadID,
		Status:         models.StatusInitialized,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	if err := s.cache.Save(ctx, meta); err != nil {
		// Clean up the orphaned MinIO upload
		_ = s.storage.AbortMultipartUpload(ctx, objectKey, minioUploadID)
		return nil, fmt.Errorf("save metadata: %w", err)
	}

	return &models.InitUploadResponse{
		UploadID:    uploadID,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
	}, nil
}

// ─── Stage 2 – Upload Chunk ──────────────────────────────────────────────────

// UploadChunk streams a single chunk to MinIO and records progress in Redis.
// chunkIndex is 0-based; S3 part numbers are 1-based.
func (s *Service) UploadChunk(ctx context.Context, uploadID string, chunkIndex int, chunkSize int64, body io.Reader) (*models.ChunkUploadResponse, error) {
	meta, err := s.getOrFail(ctx, uploadID)
	if err != nil {
		return nil, err
	}

	if chunkIndex < 0 || chunkIndex >= meta.TotalChunks {
		return nil, fmt.Errorf("chunk index %d out of range [0, %d)", chunkIndex, meta.TotalChunks)
	}

	// Skip if already uploaded (idempotent)
	for _, c := range meta.UploadedChunks {
		if c == chunkIndex {
			return &models.ChunkUploadResponse{
				UploadID:       uploadID,
				ChunkIndex:     chunkIndex,
				UploadedChunks: len(meta.UploadedChunks),
				TotalChunks:    meta.TotalChunks,
			}, nil
		}
	}

	objectKey := objectKeyFor(uploadID, meta.FileName)
	partNumber := int32(chunkIndex + 1) // S3 is 1-indexed

	if _, err := s.storage.UploadPart(ctx, objectKey, meta.MinioUploadID, partNumber, body, chunkSize); err != nil {
		return nil, fmt.Errorf("stream chunk to MinIO: %w", err)
	}

	if err := s.cache.MarkChunkDone(ctx, uploadID, chunkIndex); err != nil {
		return nil, fmt.Errorf("record chunk progress: %w", err)
	}

	// Re-fetch for accurate count
	updated, _ := s.cache.Get(ctx, uploadID)
	uploadedCount := len(meta.UploadedChunks) + 1
	if updated != nil {
		uploadedCount = len(updated.UploadedChunks)
	}

	return &models.ChunkUploadResponse{
		UploadID:       uploadID,
		ChunkIndex:     chunkIndex,
		UploadedChunks: uploadedCount,
		TotalChunks:    meta.TotalChunks,
	}, nil
}

// ─── Stage 3 – Finalise ──────────────────────────────────────────────────────

// Complete validates all chunks are present, completes the MinIO multipart
// upload, and cleans up Redis.
func (s *Service) Complete(ctx context.Context, uploadID string) (*models.CompleteUploadResponse, error) {
	meta, err := s.getOrFail(ctx, uploadID)
	if err != nil {
		return nil, err
	}

	// Validate completeness
	if len(meta.UploadedChunks) != meta.TotalChunks {
		missing := s.missingChunks(meta)
		return nil, fmt.Errorf("incomplete upload: missing chunks %v", missing)
	}

	objectKey := objectKeyFor(uploadID, meta.FileName)

	// Fetch actual ETags from MinIO
	parts, err := s.storage.ListParts(ctx, objectKey, meta.MinioUploadID)
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}

	completedParts := make([]s3types.CompletedPart, len(parts))
	for i, p := range parts {
		pCopy := p
		completedParts[i] = s3types.CompletedPart{
			PartNumber: pCopy.PartNumber,
			ETag:       pCopy.ETag,
		}
	}

	if err := s.storage.CompleteMultipartUpload(ctx, objectKey, meta.MinioUploadID, completedParts); err != nil {
		return nil, fmt.Errorf("complete MinIO upload: %w", err)
	}

	// Clean up Redis
	_ = s.cache.Delete(ctx, uploadID)

	return &models.CompleteUploadResponse{
		UploadID: uploadID,
		FileName: meta.FileName,
		Location: objectKey,
		Message:  "upload completed successfully",
	}, nil
}

// ─── Status ──────────────────────────────────────────────────────────────────

// Status returns the current progress of an upload session.
func (s *Service) Status(ctx context.Context, uploadID string) (*models.UploadStatusResponse, error) {
	meta, err := s.getOrFail(ctx, uploadID)
	if err != nil {
		return nil, err
	}

	sortedUploaded := make([]int, len(meta.UploadedChunks))
	copy(sortedUploaded, meta.UploadedChunks)
	sort.Ints(sortedUploaded)

	nextChunk := 0
	for _, c := range sortedUploaded {
		if c == nextChunk {
			nextChunk++
		}
	}

	progress := 0.0
	if meta.TotalChunks > 0 {
		progress = float64(len(sortedUploaded)) / float64(meta.TotalChunks) * 100
	}

	return &models.UploadStatusResponse{
		UploadID:        meta.UploadID,
		Status:          meta.Status,
		FileName:        meta.FileName,
		FileSize:        meta.FileSize,
		TotalChunks:     meta.TotalChunks,
		UploadedChunks:  sortedUploaded,
		NextChunkIndex:  nextChunk,
		ProgressPercent: math.Round(progress*100) / 100,
	}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) getOrFail(ctx context.Context, uploadID string) (*models.UploadMeta, error) {
	meta, err := s.cache.Get(ctx, uploadID)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	if meta == nil {
		return nil, fmt.Errorf("upload session not found or expired: %s", uploadID)
	}
	return meta, nil
}

func (s *Service) isMimeAllowed(mime string) bool {
	for _, allowed := range s.cfg.Upload.AllowedMIMETypes {
		if allowed == mime {
			return true
		}
	}
	return false
}

func (s *Service) missingChunks(meta *models.UploadMeta) []int {
	uploaded := make(map[int]bool, len(meta.UploadedChunks))
	for _, c := range meta.UploadedChunks {
		uploaded[c] = true
	}
	var missing []int
	for i := 0; i < meta.TotalChunks; i++ {
		if !uploaded[i] {
			missing = append(missing, i)
		}
	}
	return missing
}

// objectKeyFor builds a deterministic MinIO object key.
func objectKeyFor(uploadID, fileName string) string {
	ext := filepath.Ext(fileName)
	return fmt.Sprintf("uploads/%s/file%s", uploadID, ext)
}
