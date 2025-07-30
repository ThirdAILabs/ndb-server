package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"ndb-server/internal/ndb"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Version int

type Checkpointer interface {
	List() ([]Version, error)

	Download(version Version, localPath string) error

	Upload(version Version, localPath string, sources []ndb.Source) error
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

var (
	partialCheckpointRe = regexp.MustCompile(`^` + checkpointsPrefix + `/ndb_(\d+)/`)
	// The checkpoint metadata is uploaded last, so if it is missing, we can assume the checkpoint is incomplete.
	completeCheckpointRe = regexp.MustCompile(`^` + checkpointsPrefix + `/ndb_(\d+)/` + checkpointMetadataFilename + `$`)
)

type S3Checkpointer struct {
	bucket         string
	client         *s3.Client
	maxCheckpoints int
}

var _ Checkpointer = (*S3Checkpointer)(nil)

func NewS3Checkpointer(bucket, region string, maxCheckpoints int) (*S3Checkpointer, error) {
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
		bucket:         bucket,
		client:         client,
		maxCheckpoints: maxCheckpoints,
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
		match := completeCheckpointRe.FindStringSubmatch(obj)
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

	slog.Info("downloading checkpoint", "version", version, "dest", dest)

	downloader := manager.NewDownloader(c.client)

	src := filepath.Join(checkpointsPrefix, versionName(version))

	objs, err := c.listObjects(ctx, src)
	if err != nil {
		slog.Info("failed to list objects for checkpoint", "version", version, "src", src, "error", err)
		return fmt.Errorf("failed to list objects for version %d: %w", version, err)
	}

	for _, obj := range objs {
		localFilepath := filepath.Join(dest, strings.TrimPrefix(obj, src))

		if err := downloadObject(ctx, downloader, c.bucket, obj, localFilepath); err != nil {
			slog.Info("failed to list object for checkpoint", "version", version, "obj", obj, "error", err)
			return fmt.Errorf("failed to download object %s: %w", obj, err)
		}
	}

	slog.Info("checkpoint download successful", "version", version, "src", src, "dest", dest)

	return nil
}

type CheckpointMetadata struct {
	Timestamp time.Time
	Version   Version
	Documents []ndb.Source
}

const checkpointMetadataFilename = "checkpoint_metadata.json"

func (c *S3Checkpointer) Upload(version Version, src string, sources []ndb.Source) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	slog.Info("uploading checkpoint", "version", version, "src", src)

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
			slog.Info("failed to upload file", "path", path, "bucket", c.bucket, "key", key, "error", err)
			return fmt.Errorf("failed to upload file %s to s3://%s/%s: %w", path, c.bucket, key, err)
		}

		return nil
	})
	if err != nil {
		slog.Error("failed to upload checkpoint files", "src", src, "dest", dest, "error", err)
		return fmt.Errorf("failed to upload files from %s: %w", src, err)
	}

	metadata := CheckpointMetadata{
		Timestamp: time.Now(),
		Version:   version,
		Documents: sources,
	}

	metadataJson, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint metadata: %w", err)
	}

	if _, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(filepath.Join(dest, checkpointMetadataFilename)),
		Body:   bytes.NewReader(metadataJson),
	}); err != nil {
		slog.Error("failed to upload checkpoint metadata", "error", err)
		return fmt.Errorf("failed to upload checkpoint metadata to s3://%s/%s: %w", c.bucket, filepath.Join(dest, checkpointMetadataFilename), err)
	}

	if err := c.deleteOldCheckpoints(ctx); err != nil {
		slog.Error("failed to delete old checkpoints", "error", err)
		return fmt.Errorf("failed to delete old checkpoints: %w", err)
	}

	slog.Info("checkpoint upload successful", "version", version, "src", src, "dest", dest)

	return nil
}

func (c *S3Checkpointer) deleteOldCheckpoints(ctx context.Context) error {
	objs, err := c.listObjects(ctx, checkpointsPrefix)
	if err != nil {
		return fmt.Errorf("failed to list objects in bucket %s: %w", c.bucket, err)
	}

	possibleCheckpoints := make(map[Version]bool) // true means complete
	completeCheckpoints := make(map[Version]struct{})

	for _, obj := range objs {
		completeMatch := completeCheckpointRe.FindStringSubmatch(obj)
		partialMatch := partialCheckpointRe.FindStringSubmatch(obj)

		if completeMatch != nil {
			version, err := strconv.Atoi(completeMatch[1])
			if err != nil {
				return fmt.Errorf("failed to parse version from key %s: %w", obj, err)
			}
			completeCheckpoints[Version(version)] = struct{}{}
		} else if partialMatch != nil {
			version, err := strconv.Atoi(partialMatch[1])
			if err != nil {
				return fmt.Errorf("failed to parse version from key %s: %w", obj, err)
			}
			possibleCheckpoints[Version(version)] = false
		}
	}

	orderedCompleteCheckpoints := make([]Version, 0, len(completeCheckpoints))
	for version := range completeCheckpoints {
		orderedCompleteCheckpoints = append(orderedCompleteCheckpoints, version)
		possibleCheckpoints[version] = true // Mark as complete
	}
	slices.Sort(orderedCompleteCheckpoints)

	checkpointsToDelete := make([]Version, 0)
	for ckpt, complete := range possibleCheckpoints {
		if !complete {
			slog.Warn("found incomplete checkpoint", "version", ckpt)
			checkpointsToDelete = append(checkpointsToDelete, ckpt)
		}
	}

	if len(orderedCompleteCheckpoints) > c.maxCheckpoints {
		oldCheckpoints := orderedCompleteCheckpoints[:len(orderedCompleteCheckpoints)-c.maxCheckpoints]
		slog.Info("exceeded max checkpoints, deleting old checkpoints", "maxVersions", c.maxCheckpoints, "oldCheckpoints", oldCheckpoints)
		checkpointsToDelete = append(checkpointsToDelete, oldCheckpoints...)
	}

	for _, version := range checkpointsToDelete {
		slog.Info("deleting checkpoint", "version", version)
		ckptObjs, err := c.listObjects(ctx, filepath.Join(checkpointsPrefix, versionName(version)))
		if err != nil {
			slog.Error("failed to list objects for checkpoint", "version", version, "error", err)
			return fmt.Errorf("failed to list objects for version %d: %w", version, err)
		}

		deletions := make([]s3types.ObjectIdentifier, 0, len(ckptObjs))
		for _, obj := range ckptObjs {
			deletions = append(deletions, s3types.ObjectIdentifier{Key: aws.String(obj)})
		}

		if _, err := c.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &s3types.Delete{
				Objects: deletions,
			},
		}); err != nil {
			slog.Error("failed to delete objects for checkpoint", "version", version, "error", err)
			return fmt.Errorf("failed to delete objects for version %d: %w", version, err)
		}
		slog.Info("checkpoint deletion successful", "version", version)

	}

	return nil
}
