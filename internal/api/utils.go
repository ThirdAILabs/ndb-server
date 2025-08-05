package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

type codedError struct {
	err  error
	code int
}

func (e *codedError) Error() string {
	return e.err.Error()
}

func (e *codedError) Unwrap() error {
	return e.err
}

func CodedError(code int, err error) error {
	return &codedError{err: err, code: code}
}

func CodedErrorf(code int, format string, args ...any) error {
	return &codedError{err: fmt.Errorf(format, args...), code: code}
}

func ParseRequest[T any](r *http.Request) (T, error) {
	var data T
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		slog.Error("error parsing request body", "error", err)
		return data, CodedErrorf(http.StatusBadRequest, "unable to parse request body")
	}
	return data, nil
}

func RestHandler(handler func(r *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := handler(r)
		if err != nil {
			slog.Error("error handling request", "request_id", r.Context().Value(middleware.RequestIDKey), "error", err)
			var cerr *codedError
			if errors.As(err, &cerr) {
				http.Error(w, err.Error(), cerr.code)
			} else {
				slog.Error("received non coded error from endpoint", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if res == nil {
			res = struct{}{}
		}

		writeJsonResponse(w, res)
	}
}

func writeJsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		slog.Error("error serializing response body", "error", err)
		http.Error(w, fmt.Sprintf("error serializing response body: %v", err), http.StatusInternalServerError)
	}
}

func getMultipartBoundary(r *http.Request) (string, error) {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return "", CodedErrorf(http.StatusBadRequest, "missing 'Content-Type' header")
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", CodedErrorf(http.StatusBadRequest, "error parsing media type in request: %w", err)
	}
	if mediaType != "multipart/form-data" {
		return "", CodedErrorf(http.StatusBadRequest, "expected media type to be 'multipart/form-data'")
	}

	boundary, ok := params["boundary"]
	if !ok {
		return "", CodedErrorf(http.StatusBadRequest, "missing 'boundary' parameter in 'Content-Type' header")
	}

	return boundary, nil
}
