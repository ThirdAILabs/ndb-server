package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime/multipart"
	"ndb-server/internal/ndb"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const (
	maxInsertFileSize = 100 * 1024 * 1024 // 100 MB
)

type Server struct {
	lock sync.RWMutex

	ndb    ndb.NeuralDB
	leader bool

	localCheckpointDir string
	checkpointer       Checkpointer
	// currVersion is only updated with the lock held, however it is an atomic to allow
	// for checking the version while a checkpoint is being pushed/pulled in the background
	currVersion atomic.Int64
	dirty       bool
}

func (s *Server) getVersion() Version {
	return Version(s.currVersion.Load())
}

func (s *Server) setVersion(oldVersion, newVersion Version) bool {
	return s.currVersion.CompareAndSwap(int64(oldVersion), int64(newVersion))
}

func localVersionPath(dir string, version Version) string {
	return filepath.Join(dir, versionName(version))
}

func downloadLatestVersionFromCheckpointer(checkpointer Checkpointer, localCheckpointDir string) (Version, error) {
	if checkpointer == nil {
		slog.Info("no checkpointer initialized, skipping check for latest version")
		return 0, nil
	}

	checkpoints, err := checkpointer.List(slog.Default())
	if err != nil {
		return -1, fmt.Errorf("failed to list checkpoints: %w", err)
	}

	if len(checkpoints) > 0 {
		currVersion := latestVersion(checkpoints)
		localPath := localVersionPath(localCheckpointDir, currVersion)
		slog.Info("found existing checkpoints, downloading latest", "version", currVersion)

		if err := checkpointer.Download(slog.Default(), currVersion, localPath); err != nil {
			slog.Error("failed to download latest checkpoint", "version", currVersion, "error", err)
			return -1, fmt.Errorf("failed to download latest checkpoint (version=%v): %w", currVersion, err)
		}
		slog.Info("successfully downloaded latest checkpoint", "version", currVersion)
		return currVersion, nil
	} else {
		slog.Info("no checkpoints found, initializing with version 0")
		return 0, nil
	}
}

func NewServer(checkpointer Checkpointer, leader bool, localCheckpointDir string) (*Server, error) {
	if !leader && checkpointer == nil {
		return nil, fmt.Errorf("checkpointer must be initialized for non-leader nodes")
	}

	currVersion, err := downloadLatestVersionFromCheckpointer(checkpointer, localCheckpointDir)
	if err != nil {
		return nil, err
	}

	neuralDB, err := ndb.New(localVersionPath(localCheckpointDir, currVersion))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize neuralDB for version %v: %w", currVersion, err)
	}

	server := &Server{
		ndb:                neuralDB,
		leader:             leader,
		localCheckpointDir: localCheckpointDir,
		checkpointer:       checkpointer,
		dirty:              false,
	}
	server.currVersion.Store(int64(currVersion))

	return server, nil
}

func (s *Server) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/search", RestHandler(s.Search))
		r.Post("/insert", RestHandler(s.Insert))
		r.Post("/delete", RestHandler(s.Delete))
		r.Post("/upvote", RestHandler(s.Upvote))
		r.Get("/sources", RestHandler(s.Sources))
		r.Post("/checkpoint", RestHandler(s.Checkpoint))
	})

	return r
}

func formatQueryLog(query string) string {
	if len(query) > 30 {
		return query[:30] + "..."
	}
	return query
}

func (s *Server) Search(r *http.Request) (any, error) {
	searchParams, err := ParseRequest[NDBSearchParams](r)
	if err != nil {
		return nil, err
	}

	logger := slog.With("request_id", r.Context().Value(middleware.RequestIDKey), "action", "search")

	s.lock.RLock()
	defer s.lock.RUnlock()

	ndbConstaints := make(ndb.Constraints, len(searchParams.Constraints))
	for key, constraint := range searchParams.Constraints {
		constraint, err := constraint.asNDBConstraint()
		if err != nil {
			return nil, CodedErrorf(http.StatusUnprocessableEntity, "unable to parse constraint %s: %w", key, err)
		}
		ndbConstaints[key] = constraint
	}

	logger.Info("search: received", "query", formatQueryLog(searchParams.Query), "top_k", searchParams.TopK, "constraints", ndbConstaints.String())

	chunks, err := s.ndb.Query(searchParams.Query, searchParams.TopK, ndbConstaints)
	if err != nil {
		logger.Error("search: error", "error", err, "query", searchParams.Query)
		return nil, CodedErrorf(http.StatusInternalServerError, "ndb query error %w", err)
	}

	logger.Info("search: complete", "n_chunks", len(chunks))

	references := make([]Reference, len(chunks))
	for i, chunk := range chunks {
		references[i] = Reference{
			Id:       chunk.Id,
			Text:     chunk.Text,
			Source:   chunk.Document,
			SourceId: chunk.DocId,
			Metadata: chunk.Metadata,
			Score:    chunk.Score,
		}
	}

	return NDBSearchResponse{Query: searchParams.Query, References: references}, nil
}

func getMetadataAndContent(r *http.Request) ([]byte, NDBDocumentMetadata, error) {
	boundary, err := getMultipartBoundary(r)
	if err != nil {
		return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusBadRequest, "error getting multipart boundary: %w", err)
	}

	reader := multipart.NewReader(r.Body, boundary)

	var contents []byte
	var metadata NDBDocumentMetadata
	foundMetadata := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusBadRequest, "error parsing multipart request: %w", err)
		}
		defer part.Close()

		if part.FormName() == "file" {
			data, err := io.ReadAll(io.LimitReader(part, maxInsertFileSize+1)) // +1 to ensure we catch overflows
			if err != nil {
				return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusBadRequest, "error reading file: %w", err)
			}

			if len(data) > maxInsertFileSize {
				return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusUnprocessableEntity, "file size exceeds maximum limit of %d bytes, please chunk file", maxInsertFileSize)
			}

			contents = data
		} else if part.FormName() == "metadata" {
			if err := json.NewDecoder(part).Decode(&metadata); err != nil {
				return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusBadRequest, "error parsing metadata: %w", err)
			}
			foundMetadata = true
		}
	}

	if len(contents) == 0 {
		return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusBadRequest, "no file content provided")
	}
	if !foundMetadata {
		return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusBadRequest, "no metadata provided")
	}

	return contents, metadata, nil
}

func (s *Server) Insert(r *http.Request) (any, error) {
	logger := slog.With("request_id", r.Context().Value(middleware.RequestIDKey), "action", "insert")

	if !s.leader {
		return nil, CodedErrorf(http.StatusForbidden, "only leader can insert documents")
	}

	content, metadata, err := getMetadataAndContent(r)
	if err != nil {
		logger.Error("error getting metadata and content", "error", err)
		return nil, err
	}

	if !strings.HasSuffix(metadata.Filename, ".csv") {
		return nil, CodedErrorf(http.StatusUnprocessableEntity, "only CSV files are supported for insertion")
	}

	logger.Info("insert: received", "filename", metadata.Filename, "source_id", metadata.SourceId, "text_columns", metadata.TextColumns, "metadata_dtypes", metadata.MetadataTypes)

	chunks, chunkMetadata, err := ParseContent(content, metadata.TextColumns, metadata.MetadataTypes)
	if err != nil {
		logger.Error("insert: error parsing document content", "error", err)
		return nil, err
	}

	logger.Info("insert: parsed document", "n_chunks", len(chunks))

	var docId string
	if metadata.SourceId != nil {
		docId = *metadata.SourceId
	} else {
		docId = uuid.NewString()
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.ndb.Insert(metadata.Filename, docId, chunks, chunkMetadata, nil); err != nil {
		logger.Error("insert: error", "error", err, "source_id", docId)
		return nil, CodedErrorf(http.StatusInternalServerError, "ndb insert error %w", err)
	}

	s.dirty = true

	logger.Info("insert: complete", "source_id", docId)

	return nil, nil
}

func (s *Server) Delete(r *http.Request) (any, error) {
	logger := slog.With("request_id", r.Context().Value(middleware.RequestIDKey), "action", "delete")

	deleteParams, err := ParseRequest[NDBDeleteParams](r)
	if err != nil {
		return nil, err
	}

	if !s.leader {
		return nil, CodedErrorf(http.StatusForbidden, "only leader can delete documents")
	}

	logger.Info("delete: received", "ids", deleteParams.SourceIds)

	s.lock.Lock()
	defer s.lock.Unlock()

	for _, id := range deleteParams.SourceIds {
		if err := s.ndb.Delete(id, false); err != nil {
			logger.Error("delete: error", "error", err, "id", id)
			return nil, CodedErrorf(http.StatusInternalServerError, "ndb delete error %w", err)
		}
	}

	s.dirty = true

	logger.Info("delete: complete", "ids", deleteParams.SourceIds)

	return nil, nil
}

func (s *Server) Upvote(r *http.Request) (any, error) {
	if !s.leader {
		return nil, CodedErrorf(http.StatusForbidden, "only leader can upvote")
	}

	upvoteParams, err := ParseRequest[NDBUpvoteParams](r)
	if err != nil {
		return nil, err
	}

	logger := slog.With("request_id", r.Context().Value(middleware.RequestIDKey), "action", "upvote")

	logger.Info("upvote: received", "n_queries", len(upvoteParams.QueryIdPairs))

	s.lock.Lock()
	defer s.lock.Unlock()

	queries := make([]string, len(upvoteParams.QueryIdPairs))
	labels := make([]uint64, len(upvoteParams.QueryIdPairs))
	for i, pair := range upvoteParams.QueryIdPairs {
		queries[i] = pair.QueryText
		labels[i] = pair.ReferenceId
	}

	if err := s.ndb.Finetune(queries, labels); err != nil {
		logger.Error("upvote: error", "error", err)
		return nil, CodedErrorf(http.StatusInternalServerError, "ndb upvote error %w", err)
	}

	s.dirty = true

	logger.Info("upvote: complete")

	return nil, nil
}

func (s *Server) Sources(r *http.Request) (any, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	sources, err := s.ndb.Sources()
	if err != nil {
		slog.Error("sources: error", "request_id", r.Context().Value(middleware.RequestIDKey), "action", "sources", "error", err)
		return nil, CodedErrorf(http.StatusInternalServerError, "ndb list sources error %w", err)
	}

	response := make([]NDBSource, len(sources))
	for i, src := range sources {
		response[i] = NDBSource{
			Source:   src.Document,
			SourceId: src.DocId,
			Version:  src.DocVersion,
		}
	}

	return response, nil
}

func (s *Server) Checkpoint(r *http.Request) (any, error) {
	if !s.leader {
		return nil, CodedErrorf(http.StatusForbidden, "only leader can create checkpoints")
	}

	logger := slog.With("request_id", r.Context().Value(middleware.RequestIDKey), "action", "checkpoint")

	logger.Info("checkpoint: received")

	res, err := s.PushCheckpoint(logger)
	if err != nil {
		logger.Error("checkpoint: error", "error", err)
		return nil, err
	}

	logger.Info("checkpoint: complete", "version", res.Version, "new_checkpoint", res.NewCheckpoint)

	return res, nil
}

func (s *Server) PushCheckpoint(logger *slog.Logger) (NDBCheckpointResponse, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.leader {
		return NDBCheckpointResponse{}, CodedErrorf(http.StatusForbidden, "only leader can create checkpoints")
	}

	currVersion := s.getVersion()

	if !s.dirty {
		logger.Info("checkpointer: no changes to checkpoint, skipping")
		return NDBCheckpointResponse{Version: int(currVersion), NewCheckpoint: false}, nil
	}

	if s.checkpointer == nil {
		logger.Error("checkpointer: no checkpointer initialized, cannot push checkpoint")
		return NDBCheckpointResponse{}, CodedErrorf(http.StatusInternalServerError, "checkpointer must be initialized to push checkpoints")
	}

	newVersion := currVersion + 1
	logger.Info("checkpointer: creating new checkpoint", "version", newVersion)

	newVersionPath := localVersionPath(s.localCheckpointDir, newVersion)

	if err := s.ndb.Save(newVersionPath); err != nil {
		logger.Error("checkpointer: failed to save ndb state", "version", newVersion, "error", err)
		return NDBCheckpointResponse{}, CodedError(http.StatusInternalServerError, fmt.Errorf("failed to save ndb state: %w", err))
	}

	newNdb, err := ndb.New(newVersionPath)
	if err != nil {
		logger.Error("checkpointer: failed to load ndb from new checkpoint", "version", newVersion, "error", err)
		return NDBCheckpointResponse{}, fmt.Errorf("failed to load ndb from new checkpoint (version=%v): %w", newVersion, err)
	}

	sources, err := s.ndb.Sources()
	if err != nil {
		logger.Error("checkpointer: failed to get ndb sources", "version", newVersion, "error", err)
		return NDBCheckpointResponse{}, fmt.Errorf("failed to get ndb sources: %w", err)
	}

	logger.Info("checkpointer: successfully saved ndb state", "version", newVersion, "path", localVersionPath)

	if err := s.checkpointer.Upload(logger, newVersion, newVersionPath, sources); err != nil {
		logger.Error("checkpointer: failed to upload checkpoint", "version", newVersion, "error", err)
		return NDBCheckpointResponse{}, fmt.Errorf("failed to upload checkpoint (version=%v): %w", newVersion, err)
	}

	logger.Info("checkpointer: successfully uploaded checkpoint", "version", newVersion)

	s.ndb = newNdb
	if !s.setVersion(currVersion, newVersion) {
		panic(fmt.Sprintf("failed to update version from %d to %d", currVersion, newVersion))
	}
	s.dirty = false

	if err := os.RemoveAll(localVersionPath(s.localCheckpointDir, currVersion)); err != nil {
		logger.Error("checkpointer: failed to remove old checkpoint files", "version", currVersion, "error", err)
	}

	return NDBCheckpointResponse{Version: int(newVersion), NewCheckpoint: true}, nil
}

func (s *Server) PushCheckpoints(interval time.Duration) {
	if !s.leader {
		log.Fatal("PushCheckpoints should only be called on leader")
	}

	ticker := time.Tick(interval)

	logger := slog.With("action", "push_checkpoints")

	consecutiveCheckpointFailures := 0
	for {
		select {
		case <-ticker:
			if _, err := s.PushCheckpoint(logger); err != nil {
				consecutiveCheckpointFailures++
				logger.Error("checkpointer: error creating checkpoint", "error", err, "n_consecutive_failures", consecutiveCheckpointFailures)
				if consecutiveCheckpointFailures >= 3 {
					log.Fatalf("reached %d consecutive checkpoint failures, exiting", consecutiveCheckpointFailures)
				}
			} else {
				consecutiveCheckpointFailures = 0
			}
		}
	}
}

func (s *Server) PullLatestCheckpoint(logger *slog.Logger) error {
	if s.checkpointer == nil {
		logger.Error("checkpointer: no checkpointer initialized, cannot pull latest checkpoint")
		return CodedErrorf(http.StatusInternalServerError, "checkpointer must be initialized to pull checkpoints")
	}

	if s.leader {
		logger.Error("checkpointer: PullLatestCheckpoint should not be called on leader")
		return CodedErrorf(http.StatusForbidden, "PullLatestCheckpoint should not be called on leader")
	}

	checkpoints, err := s.checkpointer.List(logger)
	if err != nil {
		logger.Error("checkpointer: error listing checkpoints", "error", err)
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	currVersion := s.getVersion()

	latest := latestVersion(checkpoints)
	if latest <= currVersion {
		logger.Info("checkpointer: no new checkpoints found", "current_version", currVersion, "latest_version", latest)
		return nil
	}

	localPath := localVersionPath(s.localCheckpointDir, latest)
	if err := s.checkpointer.Download(logger, latest, localPath); err != nil {
		logger.Error("checkpointer: failed to download checkpoint", "version", latest, "error", err)
		return fmt.Errorf("failed to download checkpoint (version=%v): %w", latest, err)
	}

	logger.Info("checkpointer: successfully downloaded new checkpoint", "version", latest)

	newNdb, err := ndb.New(localPath)
	if err != nil {
		logger.Error("checkpointer: failed to load checkpoint into ndb", "version", latest, "error", err)
		return fmt.Errorf("failed to load checkpoint into ndb (version=%v): %w", latest, err)
	}

	s.lock.Lock()

	if s.setVersion(currVersion, latest) {
		logger.Info("checkpointer: updated version", "old_version", currVersion, "new_version", latest)
		s.ndb = newNdb
		s.dirty = false // doesn't really matter since this should only be called on follower

		s.lock.Unlock()

		if err := os.RemoveAll(localVersionPath(s.localCheckpointDir, currVersion)); err != nil {
			logger.Error("checkpointer: failed to remove old checkpoint files", "version", currVersion, "error", err)
		}
		logger.Info("checkpointer: successfully loaded new checkpoint into ndb", "version", latest)
	} else {
		s.lock.Unlock()
		logger.Info("checkpointer: version update skipped due to version conflict", "old_version", currVersion, "new_version", latest, "current_version", s.getVersion())
	}

	return nil
}

func (s *Server) PullCheckpoints(interval time.Duration) {
	if s.leader {
		log.Fatal("PullCheckpoints should not be called on leader")
	}

	ticker := time.Tick(interval)

	logger := slog.With("action", "pull_checkpoints")

	for {
		select {
		case <-ticker:
			if err := s.PullLatestCheckpoint(logger); err != nil {
				logger.Error("checkpointer: error pulling latest checkpoint", "error", err)
			}
		}
	}
}
