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
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type Server struct {
	lock sync.RWMutex

	ndb    ndb.NeuralDB
	leader bool

	localCheckpointDir string
	checkpointer       Checkpointer
	currVersion        Version
	dirty              bool
}

func localVersionPath(dir string, version Version) string {
	return filepath.Join(dir, versionName(version))
}

func NewServer(checkpointer Checkpointer, leader bool, localCheckpointDir string) (*Server, error) {
	checkpoints, err := checkpointer.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}

	var currVersion Version
	if len(checkpoints) > 0 {
		currVersion = latestVersion(checkpoints)
		localPath := localVersionPath(localCheckpointDir, currVersion)
		slog.Info("found existing checkpoints, downloading latest", "version", currVersion)

		if err := checkpointer.Download(currVersion, localPath); err != nil {
			slog.Error("failed to download latest checkpoint", "version", currVersion, "error", err)
			return nil, fmt.Errorf("failed to download latest checkpoint (version=%v): %w", currVersion, err)
		}
		slog.Info("successfully downloaded latest checkpoint", "version", currVersion)
	} else {
		slog.Info("no checkpoints found, initializing with version 0")
		currVersion = Version(0)
	}

	neuralDB, err := ndb.New(localVersionPath(localCheckpointDir, currVersion))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize neuralDB for version %v: %w", currVersion, err)
	}

	return &Server{
		ndb:                neuralDB,
		leader:             leader,
		localCheckpointDir: localCheckpointDir,
		checkpointer:       checkpointer,
		currVersion:        currVersion,
	}, nil
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

	slog.Info("searching ndb", "request_id", r.Context().Value(middleware.RequestIDKey), "query", formatQueryLog(searchParams.Query), "top_k", searchParams.TopK, "constraints", ndbConstaints.String())

	chunks, err := s.ndb.Query(searchParams.Query, searchParams.TopK, ndbConstaints)
	if err != nil {
		slog.Error("ndb query error", "error", err, "query", searchParams.Query)
		return nil, CodedErrorf(http.StatusInternalServerError, "ndb query error %w", err)
	}

	slog.Info("ndb query successful", "request_id", r.Context().Value(middleware.RequestIDKey), "n_chunks", len(chunks))

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
			data, err := io.ReadAll(part)
			if err != nil {
				return nil, NDBDocumentMetadata{}, CodedErrorf(http.StatusBadRequest, "error reading file: %w", err)
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
	logger := slog.With("request_id", r.Context().Value(middleware.RequestIDKey))

	if !s.leader {
		return nil, CodedErrorf(http.StatusForbidden, "only leader can insert documents")
	}

	content, metadata, err := getMetadataAndContent(r)
	if err != nil {
		logger.Error("error getting metadata and content", "error", err)
		return nil, err
	}

	logger.Info("inserting document", "filename", metadata.Filename, "source_id", metadata.SourceId, "text_columns", metadata.TextColumns, "metadata_dtypes", metadata.MetadataTypes, "doc_metadata", metadata.DocMetadata)

	chunks, chunkMetadata, err := ParseContent(content, metadata.TextColumns, metadata.MetadataTypes, metadata.DocMetadata)
	if err != nil {
		logger.Error("error parsing document content", "error", err)
		return nil, err
	}

	logger.Info("parsed document content", "n_chunks", len(chunks))

	var docId string
	if metadata.SourceId != nil {
		docId = *metadata.SourceId
	} else {
		docId = uuid.NewString()
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.ndb.Insert(metadata.Filename, docId, chunks, chunkMetadata, nil); err != nil {
		logger.Error("ndb insert error", "error", err, "source_id", docId)
		return nil, CodedErrorf(http.StatusInternalServerError, "ndb insert error %w", err)
	}

	s.dirty = true

	logger.Info("successfully inserted document", "source_id", docId)

	return nil, nil
}

func (s *Server) Delete(r *http.Request) (any, error) {
	logger := slog.With("request_id", r.Context().Value(middleware.RequestIDKey))

	deleteParams, err := ParseRequest[NDBDeleteParams](r)
	if err != nil {
		return nil, err
	}

	if !s.leader {
		return nil, CodedErrorf(http.StatusForbidden, "only leader can delete documents")
	}

	logger.Info("deleting documents", "ids", deleteParams.SourceIds)

	s.lock.Lock()
	defer s.lock.Unlock()

	for _, id := range deleteParams.SourceIds {
		if err := s.ndb.Delete(id, false); err != nil {
			logger.Error("ndb delete error", "error", err, "id", id)
			return nil, CodedErrorf(http.StatusInternalServerError, "ndb delete error %w", err)
		}
	}

	s.dirty = true

	logger.Info("successfully deleted documents", "ids", deleteParams.SourceIds)

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

	s.lock.Lock()
	defer s.lock.Unlock()

	queries := make([]string, len(upvoteParams.TextIdPairs))
	labels := make([]uint64, len(upvoteParams.TextIdPairs))
	for i, pair := range upvoteParams.TextIdPairs {
		queries[i] = pair.QueryText
		labels[i] = pair.ReferenceId
	}

	if err := s.ndb.Finetune(queries, labels); err != nil {
		slog.Error("ndb upvote error", "request_id", r.Context().Value(middleware.RequestIDKey), "error", err)
		return nil, CodedErrorf(http.StatusInternalServerError, "ndb upvote error %w", err)
	}

	s.dirty = true

	slog.Info("successfully upvoted queries", "request_id", r.Context().Value(middleware.RequestIDKey))

	return nil, nil
}

func (s *Server) Sources(r *http.Request) (any, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	sources, err := s.ndb.Sources()
	if err != nil {
		slog.Error("ndb list sources error", "request_id", r.Context().Value(middleware.RequestIDKey), "error", err)
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

	return s.PushCheckpoint()
}

func (s *Server) PushCheckpoint() (NDBCheckpointResponse, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.leader {
		return NDBCheckpointResponse{}, CodedErrorf(http.StatusForbidden, "only leader can create checkpoints")
	}

	if !s.dirty {
		slog.Info("no changes to checkpoint, skipping")
		return NDBCheckpointResponse{Version: int(s.currVersion), NewCheckpoint: false}, nil
	}

	slog.Info("creating new checkpoint", "version", s.currVersion+1)

	newVersion := s.currVersion + 1
	localVersionPath := localVersionPath(s.localCheckpointDir, newVersion)

	if err := s.ndb.Save(localVersionPath); err != nil {
		slog.Error("failed to save ndb state", "version", newVersion, "error", err)
		return NDBCheckpointResponse{}, CodedError(http.StatusInternalServerError, fmt.Errorf("failed to save ndb state: %w", err))
	}

	sources, err := s.ndb.Sources()
	if err != nil {
		slog.Error("failed to get ndb sources", "version", newVersion, "error", err)
		return NDBCheckpointResponse{}, fmt.Errorf("failed to get ndb sources: %w", err)
	}

	slog.Info("successfully saved ndb state", "version", newVersion, "path", localVersionPath)

	if err := s.checkpointer.Upload(newVersion, localVersionPath, sources); err != nil {
		slog.Error("failed to upload checkpoint", "version", newVersion, "error", err)
		return NDBCheckpointResponse{}, fmt.Errorf("failed to upload checkpoint (version=%v): %w", newVersion, err)
	}

	slog.Info("successfully uploaded checkpoint", "version", newVersion)

	s.currVersion = newVersion
	s.dirty = false

	return NDBCheckpointResponse{Version: int(newVersion), NewCheckpoint: true}, nil
}

func (s *Server) PushCheckpoints(interval time.Duration) {
	if !s.leader {
		log.Fatal("PushCheckpoints should only be called on leader")
	}

	ticker := time.Tick(interval)

	consecutiveCheckpointFailures := 0
	for {
		select {
		case <-ticker:
			if _, err := s.PushCheckpoint(); err != nil {
				consecutiveCheckpointFailures++
				slog.Error("error creating checkpoint", "error", err)
				if consecutiveCheckpointFailures >= 3 {
					log.Fatalf("reached %d consecutive checkpoint failures, exiting", consecutiveCheckpointFailures)
				}
			} else {
				consecutiveCheckpointFailures = 0
			}
		}
	}
}

func (s *Server) PullLatestCheckpoint() error {
	checkpoints, err := s.checkpointer.List()
	if err != nil {
		slog.Error("error listing checkpoints", "error", err)
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	latest := latestVersion(checkpoints)
	if latest <= s.currVersion {
		slog.Info("no new checkpoints found", "current_version", s.currVersion, "latest_version", latest)
		return nil
	}

	localPath := localVersionPath(s.localCheckpointDir, latest)
	if err := s.checkpointer.Download(latest, localPath); err != nil {
		slog.Error("failed to download checkpoint", "version", latest, "error", err)
		return fmt.Errorf("failed to download checkpoint (version=%v): %w", latest, err)
	}

	slog.Info("successfully downloaded new checkpoint", "version", latest)

	newNdb, err := ndb.New(localPath)
	if err != nil {
		slog.Error("failed to load checkpoint into ndb", "version", latest, "error", err)
		return fmt.Errorf("failed to load checkpoint into ndb (version=%v): %w", latest, err)
	}

	if err := os.RemoveAll(localVersionPath(s.localCheckpointDir, s.currVersion)); err != nil {
		slog.Error("failed to remove old checkpoint files", "version", s.currVersion, "error", err)
	}

	s.lock.Lock()
	s.ndb = newNdb
	s.currVersion = latest
	s.lock.Unlock()

	slog.Info("successfully loaded new checkpoint into ndb", "version", latest)

	return nil
}

func (s *Server) PullCheckpoints(interval time.Duration) {
	if s.leader {
		log.Fatal("PullCheckpoints should not be called on leader")
	}

	ticker := time.Tick(interval)

	for {
		select {
		case <-ticker:
			if err := s.PullLatestCheckpoint(); err != nil {
				slog.Error("error pulling latest checkpoint", "error", err)
			}
		}
	}
}
