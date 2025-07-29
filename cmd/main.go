package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"ndb-server/internal/api"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

const checkpointInterval = 60 * time.Minute

func parseFlags() (leader bool, port int, s3Bucket string, s3Region string) {
	flag.BoolVar(&leader, "leader", false, "Run as leader")
	flag.IntVar(&port, "port", 80, "Port to run the server on")
	flag.StringVar(&s3Bucket, "s3-bucket", "", "S3 bucket name for checkpoints")
	flag.StringVar(&s3Region, "s3-region", "us-west-2", "S3 region for checkpoints")

	flag.Parse()

	if s3Bucket == "" {
		log.Fatalf("s3-bucket must be specified")
	}
	if s3Region == "" {
		log.Fatalf("s3-region must be specified")
	}

	return
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	leader, port, s3Bucket, s3Region := parseFlags()

	slog.Info("startin server", "leader", leader, "port", port, "s3Bucket", s3Bucket, "s3Region", s3Region)

	checkpointer, err := api.NewS3Checkpointer(s3Bucket, s3Region)
	if err != nil {
		log.Fatalf("Failed to create S3 checkpointer: %v", err)
	}

	server, err := api.NewServer(checkpointer, leader)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if leader {
		go server.PushCheckpoints(checkpointInterval)
	} else {
		go server.PullCheckpoints(checkpointInterval)
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		server.AddRoutes(r)
	})

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	slog.Info("server stopped")
}
