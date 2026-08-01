package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

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
