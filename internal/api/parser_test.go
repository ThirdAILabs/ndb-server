package api_test

import (
	"ndb-server/internal/api"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseContent_ValidInput(t *testing.T) {
	data := []byte(`col1,col2,col3,col4
text1,123,true,extra1
text2,456,0,extra2`)
	textCols := []string{"col1", "col4"}
	metadataTypes := map[string]string{
		"col2": api.MetadataTypeInt,
		"col3": api.MetadataTypeBool,
	}

	chunks, metadata, err := api.ParseContent(data, textCols, metadataTypes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedChunks := []string{"text1 extra1", "text2 extra2"}
	expectedMetadata := []map[string]any{
		{"col2": 123, "col3": true},
		{"col2": 456, "col3": false},
	}

	assert.Equal(t, chunks, expectedChunks)

	assert.Equal(t, metadata, expectedMetadata)
}

func TestParseContent_MissingTextColumn(t *testing.T) {
	data := []byte(`col1,col2
text1,123`)
	textCols := []string{"missingCol"}
	metadataTypes := map[string]string{}

	_, _, err := api.ParseContent(data, textCols, metadataTypes)
	if err == nil || !strings.Contains(err.Error(), "text column missingCol not found in CSV header") {
		t.Errorf("expected error for missing text column, got %v", err)
	}
}

func TestParseContent_InvalidMetadataValue(t *testing.T) {
	data := []byte(`col1,col2
text1,invalidInt`)
	textCols := []string{"col1"}
	metadataTypes := map[string]string{"col2": api.MetadataTypeInt}

	_, _, err := api.ParseContent(data, textCols, metadataTypes)
	if err == nil || !strings.Contains(err.Error(), "error parsing metadata column col2 value invalidInt") {
		t.Errorf("expected error for invalid metadata value, got %v", err)
	}
}

func TestParseContent_EmptyCSV(t *testing.T) {
	data := []byte("")
	textCols := []string{"col1"}
	metadataTypes := map[string]string{}

	_, _, err := api.ParseContent(data, textCols, metadataTypes)
	if err == nil || !strings.Contains(err.Error(), "CSV file is empty") {
		t.Errorf("expected error for empty CSV, got %v", err)
	}
}
