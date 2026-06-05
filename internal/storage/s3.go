package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"easyimage/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Client interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
}

type S3Store struct {
	source config.StorageSourceConfig
	client S3Client
}

func NewS3Store(ctx context.Context, source config.StorageSourceConfig) (*S3Store, error) {
	if source.S3Bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}
	if source.S3Region == "" {
		source.S3Region = "auto"
	}
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if source.S3Endpoint != "" {
			return aws.Endpoint{URL: source.S3Endpoint, SigningRegion: source.S3Region}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(source.S3Region),
		awsconfig.WithEndpointResolverWithOptions(resolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(source.S3AccessKeyID, source.S3AccessKeySecret, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = source.S3ForcePathStyle
	})
	return &S3Store{source: source, client: client}, nil
}

func NewS3StoreWithClient(source config.StorageSourceConfig, client S3Client) *S3Store {
	return &S3Store{source: source, client: client}
}

func ObjectKey(source config.StorageSourceConfig, relativePath string) (string, error) {
	clean := path.Clean("/" + strings.TrimLeft(relativePath, "/"))
	if clean == "/" || strings.Contains(clean, "../") {
		return "", fmt.Errorf("invalid object path")
	}
	key := strings.TrimPrefix(clean, "/")
	if strings.HasPrefix(key, "i/") {
		key = strings.TrimPrefix(key, "i/")
	}
	prefix := strings.Trim(source.S3Prefix, "/")
	if prefix != "" {
		key = prefix + "/" + key
	}
	return key, nil
}

func PublicURL(source config.StorageSourceConfig, key string) string {
	base := strings.TrimRight(source.PublicBaseURL, "/")
	if base == "" {
		base = strings.TrimRight(source.S3Endpoint, "/")
		if base != "" && source.S3Bucket != "" {
			if source.S3ForcePathStyle {
				base += "/" + source.S3Bucket
			} else {
				base = strings.Replace(base, "://", "://"+source.S3Bucket+".", 1)
			}
		}
	}
	if base == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(key, "/")
}

func ContentType(filename string) string {
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func (s *S3Store) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.source.S3Bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	return err
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.source.S3Bucket), Key: aws.String(key)})
	return err
}

func (s *S3Store) CreateMultipart(ctx context.Context, key, contentType string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.source.S3Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	if out.UploadId == nil || *out.UploadId == "" {
		return "", fmt.Errorf("s3 multipart upload id is empty")
	}
	return *out.UploadId, nil
}

func (s *S3Store) UploadPart(ctx context.Context, key, uploadID string, partNumber int32, body io.Reader, size int64) (string, error) {
	out, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(s.source.S3Bucket),
		Key:           aws.String(key),
		UploadId:      aws.String(uploadID),
		PartNumber:    aws.Int32(partNumber),
		Body:          body,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return "", err
	}
	if out.ETag == nil || *out.ETag == "" {
		return "", fmt.Errorf("s3 multipart etag is empty")
	}
	return *out.ETag, nil
}

func (s *S3Store) CompleteMultipart(ctx context.Context, key, uploadID string, parts map[int32]string) error {
	partNumbers := make([]int, 0, len(parts))
	for n := range parts {
		partNumbers = append(partNumbers, int(n))
	}
	sort.Ints(partNumbers)
	completed := make([]types.CompletedPart, 0, len(parts))
	for _, n := range partNumbers {
		pn := int32(n)
		completed = append(completed, types.CompletedPart{PartNumber: aws.Int32(pn), ETag: aws.String(parts[pn])})
	}
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(s.source.S3Bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed},
	})
	return err
}

func (s *S3Store) AbortMultipart(ctx context.Context, key, uploadID string) error {
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.source.S3Bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	return err
}
