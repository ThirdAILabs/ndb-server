package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"ndb-server/internal/api"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callBackendMethod(backend http.Handler, method string, path string, body []byte, output any, reqOpts ...func(r *http.Request)) error {

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for _, opt := range reqOpts {
		opt(req)
	}

	res := httptest.NewRecorder()
	backend.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d: '%s'", res.Code, res.Body.String())
	}

	if output != nil {
		if err := json.NewDecoder(res.Body).Decode(output); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

func callSearch(backend http.Handler, query string, topk int, constraints map[string]api.Constraint) (api.NDBSearchResponse, error) {
	request := api.NDBSearchParams{
		Query:       query,
		TopK:        topk,
		Constraints: constraints,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return api.NDBSearchResponse{}, err
	}

	var response api.NDBSearchResponse
	if err := callBackendMethod(backend, http.MethodPost, "/api/v1/search", body, &response); err != nil {
		return api.NDBSearchResponse{}, fmt.Errorf("failed to call search: %w", err)
	}

	return response, nil
}

func callInsert(backend http.Handler, data string, metdata api.NDBDocumentMetadata) error {
	body := new(bytes.Buffer)

	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "file.csv")
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write([]byte(data)); err != nil {
		return fmt.Errorf("failed to write data to form file: %w", err)
	}

	metadataJSON, err := json.Marshal(metdata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := writer.WriteField("metadata", string(metadataJSON)); err != nil {
		return fmt.Errorf("failed to write metadata to form: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	if err := callBackendMethod(backend, http.MethodPost, "/api/v1/insert", body.Bytes(), nil, func(r *http.Request) {
		r.Header.Set("Content-Type", writer.FormDataContentType())
	}); err != nil {
		return fmt.Errorf("failed to call insert: %w", err)
	}

	return nil
}

func callDelete(backend http.Handler, id string) error {
	request := api.NDBDeleteParams{
		SourceIds: []string{id},
	}

	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	if err := callBackendMethod(backend, http.MethodPost, "/api/v1/delete", body, nil); err != nil {
		return fmt.Errorf("failed to call delete: %w", err)
	}

	return nil
}

func checkResults(t *testing.T, response api.NDBSearchResponse, expectedIds []int) {
	t.Helper()

	require.Len(t, response.References, len(expectedIds), "unexpected number of results")

	for i, ref := range response.References {
		assert.Equal(t, expectedIds[i], int(ref.Id), "unexpected reference ID at index %d, got %d", i, ref.Id)
		assert.NotEmpty(t, ref.Text)
		assert.NotEmpty(t, ref.Source)
		assert.NotEmpty(t, ref.SourceId)
		assert.NotEmpty(t, ref.Metadata)
		assert.Greater(t, ref.Score, float32(0.0), "expected score to be greater than 0, got %f", ref.Score)
	}
}

const doc1 = `text,k1,k2,k3,k4
a,5.2,false,7,apple
a b,4.1,true,7,banana
a b c,9.2,false,11,kiwi
a b c d,2.9,false,7,grape
a b c d e,4.7,false,7,pineapple`

const doc2 = `text,k1,k2,k3
w,mango,2,abc
w x,mandarin,4,def
w x y,coconut,2,abc
w x y z,orange,3,ghi`

func TestLeaderOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	checkpointer, _ := createMinioS3Checkpointer(t, ctx)

	server, err := api.NewServer(checkpointer, true, t.TempDir())
	require.NoError(t, err)

	router := server.Router()

	docId1 := "doc1"

	t.Run("Insert", func(t *testing.T) {
		require.NoError(t, callInsert(router, doc1, api.NDBDocumentMetadata{
			Filename:      "file.csv",
			SourceId:      &docId1,
			TextColumns:   []string{"text"},
			MetadataTypes: map[string]string{"k1": api.MetadataTypeFloat, "k2": api.MetadataTypeBool, "k3": api.MetadataTypeInt, "k4": api.MetadataTypeString},
			DocMetadata:   map[string]any{"doc_num": "one"},
		}))

		require.NoError(t, callInsert(router, doc2, api.NDBDocumentMetadata{
			Filename:      "file.csv",
			SourceId:      nil,
			TextColumns:   []string{"text"},
			MetadataTypes: map[string]string{"k1": api.MetadataTypeString, "k2": api.MetadataTypeInt, "k3": api.MetadataTypeString},
			DocMetadata:   map[string]any{"doc_num": "two"},
		}))
	})

	t.Run("Search", func(t *testing.T) {
		res1, err := callSearch(router, "a b c d e", 10, nil)
		require.NoError(t, err)
		checkResults(t, res1, []int{4, 3, 2, 1, 0})

		res2, err := callSearch(router, "d e", 10, nil)
		require.NoError(t, err)
		checkResults(t, res2, []int{4, 3})

		res3, err := callSearch(router, "z e", 10, nil)
		require.NoError(t, err)
		checkResults(t, res3, []int{8, 4})
	})

	t.Run("Constrained Search", func(t *testing.T) {
		constraints1 := map[string]api.Constraint{
			"k1": {ConstraintType: api.GreaterThanType, Value: 3.0, Dtype: api.MetadataTypeFloat},
			"k2": {ConstraintType: api.EqualToType, Value: false, Dtype: api.MetadataTypeBool},
			"k3": {ConstraintType: api.EqualToType, Value: 7, Dtype: api.MetadataTypeInt},
			"k4": {ConstraintType: api.LessThanType, Value: "peach", Dtype: api.MetadataTypeString},
		}

		res1, err := callSearch(router, "a b c d e", 10, constraints1)
		require.NoError(t, err)
		checkResults(t, res1, []int{0})

		constraints2 := map[string]api.Constraint{
			"k4": {ConstraintType: api.AnyOfType, Value: []any{"apple", "banana"}, Dtype: api.MetadataTypeString},
		}

		res2, err := callSearch(router, "a b c d e", 10, constraints2)
		require.NoError(t, err)
		checkResults(t, res2, []int{1, 0})

		constraints3 := map[string]api.Constraint{
			"k4": {ConstraintType: api.SubstringType, Value: "apple", Dtype: api.MetadataTypeString},
		}

		res3, err := callSearch(router, "a b c d e", 10, constraints3)
		require.NoError(t, err)
		checkResults(t, res3, []int{4, 0})

		constraints4 := map[string]api.Constraint{
			"k4": {ConstraintType: api.SubstringType, Value: "apple", Dtype: api.MetadataTypeString},
			"k1": {ConstraintType: api.AnyOfType, Value: []any{3.1, 2.9, 4.7}, Dtype: api.MetadataTypeFloat},
		}

		res4, err := callSearch(router, "a b c d e", 10, constraints4)
		require.NoError(t, err)
		checkResults(t, res4, []int{4})
	})

	t.Run("Delete", func(t *testing.T) {
		res1, err := callSearch(router, "z e", 10, nil)
		require.NoError(t, err)
		checkResults(t, res1, []int{8, 4})

		require.NoError(t, callDelete(router, docId1))

		res2, err := callSearch(router, "z e", 10, nil)
		require.NoError(t, err)
		checkResults(t, res2, []int{8})
	})
}

func TestLeaderAndFollower(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	checkpointer, _ := createMinioS3Checkpointer(t, ctx)

	leader, err := api.NewServer(checkpointer, true, t.TempDir())
	require.NoError(t, err)
	leaderRouter := leader.Router()

	require.NoError(t, callInsert(leaderRouter, doc1, api.NDBDocumentMetadata{
		Filename:      "file.csv",
		SourceId:      nil,
		TextColumns:   []string{"text"},
		MetadataTypes: map[string]string{"k1": api.MetadataTypeFloat, "k2": api.MetadataTypeBool, "k3": api.MetadataTypeInt, "k4": api.MetadataTypeString},
		DocMetadata:   map[string]any{"doc_num": "one"},
	}))

	ckpt, err := leader.PushCheckpoint()
	require.NoError(t, err)
	assert.Equal(t, ckpt.Version, 1)
	assert.Equal(t, ckpt.NewCheckpoint, true)

	follower, err := api.NewServer(checkpointer, false, t.TempDir())
	require.NoError(t, err)
	followerRouter := follower.Router()

	t.Run("Search Checkpoint 1", func(t *testing.T) {
		for _, router := range []http.Handler{leaderRouter, followerRouter} {
			constraints1 := map[string]api.Constraint{
				"k1": {ConstraintType: api.GreaterThanType, Value: 3.0, Dtype: api.MetadataTypeFloat},
				"k2": {ConstraintType: api.EqualToType, Value: false, Dtype: api.MetadataTypeBool},
				"k3": {ConstraintType: api.EqualToType, Value: 7, Dtype: api.MetadataTypeInt},
				"k4": {ConstraintType: api.LessThanType, Value: "peach", Dtype: api.MetadataTypeString},
			}

			res1, err := callSearch(router, "a b c d e", 10, constraints1)
			require.NoError(t, err)
			checkResults(t, res1, []int{0})

			res2, err := callSearch(router, "z e", 10, nil)
			require.NoError(t, err)
			checkResults(t, res2, []int{4})
		}
	})

	t.Run("Update Checkpoint", func(t *testing.T) {
		require.NoError(t, callInsert(leaderRouter, doc2, api.NDBDocumentMetadata{
			Filename:      "file.csv",
			SourceId:      nil,
			TextColumns:   []string{"text"},
			MetadataTypes: map[string]string{"k1": api.MetadataTypeString, "k2": api.MetadataTypeInt, "k3": api.MetadataTypeString},
			DocMetadata:   map[string]any{"doc_num": "two"},
		}))

		ckpt, err := leader.PushCheckpoint()
		require.NoError(t, err)
		assert.Equal(t, ckpt.Version, 2)
		assert.Equal(t, ckpt.NewCheckpoint, true)

		require.NoError(t, follower.PullLatestCheckpoint())
	})

	t.Run("Search Checkpoint 2", func(t *testing.T) {
		for _, router := range []http.Handler{leaderRouter, followerRouter} {
			constraints1 := map[string]api.Constraint{
				"k1": {ConstraintType: api.GreaterThanType, Value: 3.0, Dtype: api.MetadataTypeFloat},
				"k2": {ConstraintType: api.EqualToType, Value: false, Dtype: api.MetadataTypeBool},
				"k3": {ConstraintType: api.EqualToType, Value: 7, Dtype: api.MetadataTypeInt},
				"k4": {ConstraintType: api.LessThanType, Value: "peach", Dtype: api.MetadataTypeString},
			}

			res1, err := callSearch(router, "a b c d e", 10, constraints1)
			require.NoError(t, err)
			checkResults(t, res1, []int{0})

			res2, err := callSearch(router, "z e", 10, nil)
			require.NoError(t, err)
			checkResults(t, res2, []int{8, 4})

			constraints3 := map[string]api.Constraint{
				"k1": {ConstraintType: api.SubstringType, Value: "man", Dtype: api.MetadataTypeString},
				"k3": {ConstraintType: api.AnyOfType, Value: []any{"abc", "ghi"}, Dtype: api.MetadataTypeString},
			}

			res3, err := callSearch(router, "w x y z a b", 10, constraints3)
			require.NoError(t, err)
			checkResults(t, res3, []int{5})
		}
	})
}
