package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Version int

type Checkpointer interface {
	List() ([]Version, error)

	Download(version Version, localPath string) error

	Upload(version Version, localPath string) error
}

func versionName(version Version) string {
	return fmt.Sprintf("ndb_%d", version)
}

func localVersionPath(version Version) string {
	const localCheckpoints = "./checkpoints"
	return filepath.Join(localCheckpoints, versionName(version))
}

func latestVersion(versions []Version) Version {
	max := Version(-1)
	for _, v := range versions {
		if v > max {
			max = v
		}
	}
	return max
}

const checkpointsPrefix = "checkpoints"

var checkpointRe = regexp.MustCompile(`^checkpoints/ndb_(\d+)$`)

type S3Checkpointer struct {
	bucket string
	client *s3.Client
}

var _ Checkpointer = (*S3Checkpointer)(nil)

func NewS3Checkpointer(bucket, region string) (*S3Checkpointer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	opts := []func(*aws_config.LoadOptions) error{}
	if region != "" {
		opts = append(opts, aws_config.WithRegion(region))
	}

	cfg, err := aws_config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// Needed for MinIO which doesn't enforce bucket naming rules always
		o.UsePathStyle = true // Use path-style addressing (needed for MinIO) - Assuming true based on original, not cfg.S3UsePathStyle
	})

	return &S3Checkpointer{
		bucket: bucket,
		client: client,
	}, nil
}

func (c *S3Checkpointer) listObjects(ctx context.Context, prefix string) ([]string, error) {
	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefix),
	})

	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects in bucket %s: %w", c.bucket, err)
		}

		for _, obj := range page.Contents {
			keys = append(keys, *obj.Key)
		}
	}

	return keys, nil
}

func (c *S3Checkpointer) List() ([]Version, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	objects, err := c.listObjects(ctx, checkpointsPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in bucket %s: %w", c.bucket, err)
	}

	versions := make([]Version, 0)

	for _, obj := range objects {
		match := checkpointRe.FindStringSubmatch(obj)
		if match != nil {
			version, err := strconv.Atoi(match[1])
			if err != nil {
				return nil, fmt.Errorf("failed to parse version from key %s: %w", obj, err)
			}
			versions = append(versions, Version(version))
		}
	}

	return versions, nil
}

func downloadObject(ctx context.Context, downloader *manager.Downloader, bucket, key, filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory for download %s: %w", filepath.Dir(filename), err)
	}
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	_, err = downloader.Download(ctx, file, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download object %s from s3://%s/%s: %w", filename, bucket, key, err)
	}
	slog.Info("object downloaded successfully", "bucket", bucket, "key", key)

	return nil
}

func (c *S3Checkpointer) Download(version Version, dest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	downloader := manager.NewDownloader(c.client)

	src := filepath.Join(checkpointsPrefix, versionName(version))

	objs, err := c.listObjects(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to list objects for version %d: %w", version, err)
	}

	for _, obj := range objs {
		localFilepath := filepath.Join(dest, strings.TrimPrefix(obj, src))

		if err := downloadObject(ctx, downloader, c.bucket, obj, localFilepath); err != nil {
			return fmt.Errorf("failed to download object %s: %w", obj, err)
		}
	}

	return nil
}

func (c *S3Checkpointer) Upload(version Version, src string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dest := filepath.Join(checkpointsPrefix, versionName(version))

	uploader := manager.NewUploader(c.client)

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to walk directory %s: %w", src, err)
		}

		if info.IsDir() {
			return nil
		}

		key := filepath.Join(dest, strings.TrimPrefix(path, src))

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		if _, err := uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
			Body:   file,
		}); err != nil {
			return fmt.Errorf("failed to upload file %s to s3://%s/%s: %w", path, c.bucket, key, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to upload files from %s: %w", src, err)
	}

	return nil
}
