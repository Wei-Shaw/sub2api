package repository

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestS3ImageStorageLoadUsesPrivateGetObject(t *testing.T) {
	var gotBucket, gotKey string
	storage := &S3ImageStorage{
		bucket: "image-bucket",
		getObject: func(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			gotBucket = valueOrEmpty(input.Bucket)
			gotKey = valueOrEmpty(input.Key)
			contentType := "application/octet-stream"
			return &s3.GetObjectOutput{
				Body: io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\npayload"))), ContentType: &contentType,
			}, nil
		},
	}

	data, contentType, err := storage.Load(context.Background(), "images/task-0.png", 1024)
	require.NoError(t, err)
	require.Equal(t, "image-bucket", gotBucket)
	require.Equal(t, "images/task-0.png", gotKey)
	require.Equal(t, "image/png", contentType)
	require.NotEmpty(t, data)
}

func TestS3ImageStorageLoadRejectsOversizedAndNonImageObjects(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
		maxBytes    int64
		wantError   string
	}{
		{name: "oversized", body: []byte("12345"), contentType: "image/png", maxBytes: 4, wantError: "exceeds 4 bytes"},
		{name: "not image", body: []byte("plain text"), contentType: "text/plain", maxBytes: 1024, wantError: "is not an image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &S3ImageStorage{
				bucket: "image-bucket",
				getObject: func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(tt.body)), ContentType: &tt.contentType}, nil
				},
			}
			_, _, err := storage.Load(context.Background(), "images/object", tt.maxBytes)
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}

func TestS3ImageStorageLoadPropagatesGetFailure(t *testing.T) {
	storage := &S3ImageStorage{
		bucket: "image-bucket",
		getObject: func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, errors.New("access denied")
		},
	}
	_, _, err := storage.Load(context.Background(), "images/object", 1024)
	require.ErrorContains(t, err, "access denied")
}

func TestS3ImageStorageSizeUsesConfiguredBucketAndKey(t *testing.T) {
	var gotBucket, gotKey string
	storage := &S3ImageStorage{
		bucket: "image-bucket",
		headObject: func(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			gotBucket = valueOrEmpty(input.Bucket)
			gotKey = valueOrEmpty(input.Key)
			contentLength := int64(1536)
			return &s3.HeadObjectOutput{ContentLength: &contentLength}, nil
		},
	}

	size, err := storage.Size(context.Background(), "images/imgtask_1-0.png")
	require.NoError(t, err)
	require.Equal(t, int64(1536), size)
	require.Equal(t, "image-bucket", gotBucket)
	require.Equal(t, "images/imgtask_1-0.png", gotKey)
}

func TestS3ImageStorageDeleteUsesConfiguredBucketAndKey(t *testing.T) {
	var gotBucket, gotKey string
	storage := &S3ImageStorage{
		bucket: "image-bucket",
		deleteObject: func(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			gotBucket = valueOrEmpty(input.Bucket)
			gotKey = valueOrEmpty(input.Key)
			return &s3.DeleteObjectOutput{}, nil
		},
	}

	require.NoError(t, storage.Delete(context.Background(), "images/imgtask_1-0.png"))
	require.Equal(t, "image-bucket", gotBucket)
	require.Equal(t, "images/imgtask_1-0.png", gotKey)
}

func TestS3ImageStorageDeletePropagatesFailure(t *testing.T) {
	storage := &S3ImageStorage{
		bucket: "image-bucket",
		deleteObject: func(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			return nil, errors.New("access denied")
		},
	}

	err := storage.Delete(context.Background(), "images/imgtask_1-0.png")
	require.ErrorContains(t, err, "access denied")
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
