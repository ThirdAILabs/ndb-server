package api

import (
	"fmt"
	"log/slog"
	"ndb-server/internal/ndb"
)

type Version int

type Checkpointer interface {
	List(logger *slog.Logger) ([]Version, error)

	Download(logger *slog.Logger, version Version, localPath string) error

	Upload(logger *slog.Logger, version Version, localPath string, sources []ndb.Source) error
}

func versionName(version Version) string {
	return fmt.Sprintf("ndb_%d", version)
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
