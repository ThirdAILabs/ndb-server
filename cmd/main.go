package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"ndb-server/internal/api"
	"net/http"
	"time"
)

type config struct {
	leader             bool
	port               int
	s3Bucket           string
	s3Region           string
	maxCheckpoints     int
	localCheckpointDir string
	checkpointInterval time.Duration
}

func parseFlags() config {
	var cfg config
	var checkpointIntervalStr string

	flag.BoolVar(&cfg.leader, "leader", false, "Run as leader")
	flag.IntVar(&cfg.port, "port", 80, "Port to run the server on")
	flag.StringVar(&cfg.s3Bucket, "s3-bucket", "", "S3 bucket name for checkpoints, optional if not using no checkpoints will be pushed")
	flag.StringVar(&cfg.s3Region, "s3-region", "", "S3 region for checkpoints, required if s3-bucket is specified")
	flag.IntVar(&cfg.maxCheckpoints, "max-ckpts", 10, "Maximum number of checkpoints to keep in S3")
	flag.StringVar(&cfg.localCheckpointDir, "ckpt-dir", "./checkpoints", "Local directory to store checkpoints")
	flag.StringVar(&checkpointIntervalStr, "ckpt-interval", "1h", "Interval for checkpoints (e.g., 5m, 1h5m)")

	flag.Parse()

	if cfg.s3Bucket != "" && cfg.s3Region == "" {
		log.Fatalf("s3-region must be specified")
	}

	checkpointInterval, err := time.ParseDuration(checkpointIntervalStr)
	if err != nil {
		log.Fatalf("Invalid checkpoint interval: %v", err)
	}
	cfg.checkpointInterval = checkpointInterval

	return cfg
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg := parseFlags()

	slog.Info("config", "leader", cfg.leader, "port", cfg.port,
		"s3Bucket", cfg.s3Bucket, "s3Region", cfg.s3Region,
		"maxCheckpoints", cfg.maxCheckpoints, "localCheckpointDir", cfg.localCheckpointDir,
		"checkpointInterval", cfg.checkpointInterval.String(),
	)

	var checkpointer api.Checkpointer
	if cfg.s3Bucket != "" {
		var err error
		checkpointer, err = api.NewS3Checkpointer(cfg.s3Bucket, cfg.s3Region, cfg.maxCheckpoints)
		if err != nil {
			log.Fatalf("Failed to create S3 checkpointer: %v", err)
		}
	} else {
		slog.Info("no s3 bucket specified, no checkpoints will be saved")
	}

	server, err := api.NewServer(checkpointer, cfg.leader, cfg.localCheckpointDir)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if cfg.leader {
		go server.PushCheckpoints(cfg.checkpointInterval)
	} else {
		go server.PullCheckpoints(cfg.checkpointInterval)
	}

	router := server.Router()

	slog.Info("starting server")
	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.port), router); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}

	slog.Info("server stopped")
}
