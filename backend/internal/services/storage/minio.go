package storage

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/yourorg/chunked-upload/internal/config"
)

// Service wraps AWS S3 SDK v2 configured for MinIO.
type Service struct {
	client *s3.Client
	bucket string
}

// New creates an S3 client pointing at the configured MinIO endpoint.
func New(cfg config.MinIOConfig) (*Service, error) {
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	endpoint := fmt.Sprintf("%s://%s", scheme, cfg.Endpoint)

	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               endpoint,
				HostnameImmutable: true,
				SigningRegion:     "us-east-1",
			}, nil
		},
	)

	awsCfg, err := awsConfig.LoadDefaultConfig(
		context.Background(),
		awsConfig.WithRegion("us-east-1"),
		awsConfig.WithEndpointResolverWithOptions(customResolver),
		awsConfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true // required for MinIO
	})

	svc := &Service{client: client, bucket: cfg.Bucket}
	if err := svc.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	return svc, nil
}

// ensureBucket creates the bucket if it doesn't exist.
func (s *Service) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		_, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(s.bucket),
		})
		if createErr != nil && !strings.Contains(createErr.Error(), "BucketAlreadyOwnedByYou") {
			return fmt.Errorf("create bucket: %w", createErr)
		}
	}
	return nil
}

// CreateMultipartUpload initiates an S3 multipart upload and returns the UploadId.
func (s *Service) CreateMultipartUpload(ctx context.Context, objectKey, contentType string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("create multipart upload: %w", err)
	}
	return aws.ToString(out.UploadId), nil
}

// UploadPart streams a single chunk to S3 and returns the ETag.
// partNumber is 1-indexed (S3 requirement).
func (s *Service) UploadPart(ctx context.Context, objectKey, minioUploadID string, partNumber int32, body io.Reader, size int64) (string, error) {
	out, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(objectKey),
		UploadId:      aws.String(minioUploadID),
		PartNumber:    aws.Int32(partNumber),
		Body:          body,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return "", fmt.Errorf("upload part %d: %w", partNumber, err)
	}
	return aws.ToString(out.ETag), nil
}

// CompleteMultipartUpload finalises the multipart upload using the provided ETags.
// completedParts must be sorted by PartNumber ascending.
func (s *Service) CompleteMultipartUpload(ctx context.Context, objectKey, minioUploadID string, completedParts []types.CompletedPart) error {
	sort.Slice(completedParts, func(i, j int) bool {
		return aws.ToInt32(completedParts[i].PartNumber) < aws.ToInt32(completedParts[j].PartNumber)
	})

	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(minioUploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}
	return nil
}

// AbortMultipartUpload cancels a multipart upload and frees staging storage.
func (s *Service) AbortMultipartUpload(ctx context.Context, objectKey, minioUploadID string) error {
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(minioUploadID),
	})
	return err
}

// ListParts returns the list of already-uploaded parts so we can build the
// CompletedParts list during finalisation.
func (s *Service) ListParts(ctx context.Context, objectKey, minioUploadID string) ([]types.Part, error) {
	var parts []types.Part
	var marker *string

	for {
		out, err := s.client.ListParts(ctx, &s3.ListPartsInput{
			Bucket:           aws.String(s.bucket),
			Key:              aws.String(objectKey),
			UploadId:         aws.String(minioUploadID),
			PartNumberMarker: marker,
		})
		if err != nil {
			return nil, fmt.Errorf("list parts: %w", err)
		}
		parts = append(parts, out.Parts...)
		if !aws.ToBool(out.IsTruncated) {
			break
		}
		if len(out.Parts) > 0 {
			last := out.Parts[len(out.Parts)-1]
			marker = aws.String(fmt.Sprintf("%d", aws.ToInt32(last.PartNumber)))
		}
	}
	return parts, nil
}
