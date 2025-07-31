package api

import (
	"fmt"
	"ndb-server/internal/ndb"
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

func latestVersion(versions []Version) Version {
	max := Version(-1)
	for _, v := range versions {
		if v > max {
			max = v
		}
	}
	return max
}
