package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourorg/chunked-upload/internal/config"
	"github.com/yourorg/chunked-upload/internal/models"
)

const keyPrefix = "upload:"

// Service provides Redis-backed persistence for upload metadata.
type Service struct {
	client *redis.Client
	ttl    time.Duration
}

// New creates a Redis client and verifies the connection.
func New(cfg config.RedisConfig, ttl time.Duration) (*Service, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Service{client: client, ttl: ttl}, nil
}

// key returns the namespaced Redis key for an upload ID.
func key(uploadID string) string {
	return keyPrefix + uploadID
}

// Save serialises UploadMeta to Redis with a rolling TTL.
func (s *Service) Save(ctx context.Context, meta *models.UploadMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal upload meta: %w", err)
	}
	return s.client.Set(ctx, key(meta.UploadID), data, s.ttl).Err()
}

// Get retrieves and deserialises UploadMeta. Returns (nil, nil) when not found.
func (s *Service) Get(ctx context.Context, uploadID string) (*models.UploadMeta, error) {
	data, err := s.client.Get(ctx, key(uploadID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var meta models.UploadMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal upload meta: %w", err)
	}
	return &meta, nil
}

// MarkChunkDone appends chunkIndex to the uploaded list and refreshes TTL.
func (s *Service) MarkChunkDone(ctx context.Context, uploadID string, chunkIndex int) error {
	meta, err := s.Get(ctx, uploadID)
	if err != nil {
		return err
	}
	if meta == nil {
		return fmt.Errorf("upload not found: %s", uploadID)
	}

	// Deduplicate
	for _, c := range meta.UploadedChunks {
		if c == chunkIndex {
			return nil // already recorded
		}
	}

	meta.UploadedChunks = append(meta.UploadedChunks, chunkIndex)
	meta.Status = models.StatusInProgress
	meta.UpdatedAt = time.Now().UTC()

	return s.Save(ctx, meta)
}

// Delete removes the upload record from Redis (called after successful merge).
func (s *Service) Delete(ctx context.Context, uploadID string) error {
	return s.client.Del(ctx, key(uploadID)).Err()
}

// Close closes the Redis connection.
func (s *Service) Close() error {
	return s.client.Close()
}
