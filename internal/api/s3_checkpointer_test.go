package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"ndb-server/internal/api"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/minio"
)

const bucketName = "test-bucket"

func createMinioS3Checkpointer(t *testing.T, ctx context.Context) (api.Checkpointer, *s3.Client) {
	t.Helper()

	const (
		minioUsername = "minioadmin"
		minioPassword = "miniopassword"
	)

	minioContainer, err := minio.Run(
		ctx,
		"minio/minio:RELEASE.2024-01-16T16-07-38Z",
		minio.WithUsername(minioUsername),
		minio.WithPassword(minioPassword),
	)
	require.NoError(t, err, "Failed to start MinIO container")

	t.Cleanup(func() {
		err := minioContainer.Terminate(context.Background())
		require.NoError(t, err, "Failed to terminate MinIO container")
	})

	connStr, err := minioContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get MinIO connection string")

	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) { // nolint:staticcheck
		return aws.Endpoint{ // nolint:staticcheck
			PartitionID:       "aws",
			URL:               "http://" + connStr,
			SigningRegion:     "",
			HostnameImmutable: true, // Important for MinIO
		}, nil
	})

	credentials := credentials.NewStaticCredentialsProvider(minioUsername, minioPassword, "")

	s3cfg, err := aws_config.LoadDefaultConfig(context.Background(), aws_config.WithCredentialsProvider(credentials), aws_config.WithEndpointResolverWithOptions(resolver))
	require.NoError(t, err, "Failed to load AWS config for MinIO")

	client := s3.NewFromConfig(s3cfg, func(o *s3.Options) {
		o.UsePathStyle = true // Important for MinIO
	})

	_, err = client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err, "Failed to create bucket in MinIO")

	return api.NewS3CheckpointerFromClient(client, bucketName, 3), client
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755), "Failed to create directory for file")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644), "Failed to write file")
}

func assertSameFileContent(t *testing.T, path1, path2 string) {
	t.Helper()

	content1, err := os.ReadFile(path1)
	require.NoError(t, err, "Failed to read file 1")
	content2, err := os.ReadFile(path2)
	require.NoError(t, err, "Failed to read file 2")

	assert.Equal(t, content1, content2, fmt.Sprintf("File contents do not match: '%s' vs '%s'", content1, content2))
}

func TestS3Checkpointer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	checkpointer, s3client := createMinioS3Checkpointer(t, ctx)

	localDir := t.TempDir()

	t.Run("Upload and Download Checkpoints", func(t *testing.T) {
		writeFile(t, filepath.Join(localDir, "0/file.txt"), "checkpoint 0 data")
		writeFile(t, filepath.Join(localDir, "1/file.txt"), "checkpoint 1 data")
		writeFile(t, filepath.Join(localDir, "2/file.txt"), "checkpoint 2 data")

		assert.NoError(t, checkpointer.Upload(slog.Default(), api.Version(0), filepath.Join(localDir, "0"), nil))
		assert.NoError(t, checkpointer.Upload(slog.Default(), api.Version(1), filepath.Join(localDir, "1"), nil))
		assert.NoError(t, checkpointer.Upload(slog.Default(), api.Version(2), filepath.Join(localDir, "2"), nil))

		assert.NoError(t, checkpointer.Download(slog.Default(), api.Version(0), filepath.Join(localDir, "0_download")))
		assert.NoError(t, checkpointer.Download(slog.Default(), api.Version(1), filepath.Join(localDir, "1_download")))
		assert.NoError(t, checkpointer.Download(slog.Default(), api.Version(2), filepath.Join(localDir, "2_download")))

		assertSameFileContent(t, filepath.Join(localDir, "0_download", "file.txt"), filepath.Join(localDir, "0", "file.txt"))
		assertSameFileContent(t, filepath.Join(localDir, "1_download", "file.txt"), filepath.Join(localDir, "1", "file.txt"))
		assertSameFileContent(t, filepath.Join(localDir, "2_download", "file.txt"), filepath.Join(localDir, "2", "file.txt"))

		ckpts, err := checkpointer.List(slog.Default())
		require.NoError(t, err)
		assert.ElementsMatch(t, []api.Version{0, 1, 2}, ckpts)
	})

	t.Run("Max Checkpoints", func(t *testing.T) {
		writeFile(t, filepath.Join(localDir, "3/file.txt"), "checkpoint 3 data")

		assert.NoError(t, checkpointer.Upload(slog.Default(), api.Version(3), filepath.Join(localDir, "3"), nil))
		assert.NoError(t, checkpointer.Download(slog.Default(), api.Version(3), filepath.Join(localDir, "3_download")))
		assertSameFileContent(t, filepath.Join(localDir, "3_download", "file.txt"), filepath.Join(localDir, "3", "file.txt"))

		ckpts, err := checkpointer.List(slog.Default())
		require.NoError(t, err)
		assert.ElementsMatch(t, []api.Version{1, 2, 3}, ckpts)
	})

	t.Run("Delete Incomplete Checkpoint", func(t *testing.T) {
		_, delErr := s3client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String("checkpoints/ndb_2/checkpoint_metadata.json"),
		}) // Delete checkpoint metadata to simulate an incomplete checkpoint
		require.NoError(t, delErr)

		_, head1Err := s3client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String("checkpoints/ndb_2/file.txt"),
		}) // Incomplete checkpoint will not be removed until next upload
		require.NoError(t, head1Err)

		ckpts1, err := checkpointer.List(slog.Default())
		require.NoError(t, err)
		assert.ElementsMatch(t, []api.Version{1, 3}, ckpts1)

		writeFile(t, filepath.Join(localDir, "4/file.txt"), "checkpoint 4 data")
		assert.NoError(t, checkpointer.Upload(slog.Default(), api.Version(4), filepath.Join(localDir, "4"), nil))

		_, head2Err := s3client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String("checkpoints/ndb_2/file.txt"),
		}) // After upload the incomplete checkpoint should be removed
		require.Error(t, head2Err)

		ckpts2, err := checkpointer.List(slog.Default())
		require.NoError(t, err)
		assert.ElementsMatch(t, []api.Version{1, 3, 4}, ckpts2)
	})

}
