package api

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type metadataParserFunc func(string) (any, error)

func buildMetadataParsers(colToIdx map[string]int, metadataTypes map[string]string) (map[string]metadataParserFunc, error) {
	metdataParsers := make(map[string]metadataParserFunc)
	for col, dtype := range metadataTypes {
		if _, ok := colToIdx[col]; !ok {
			return nil, CodedErrorf(http.StatusUnprocessableEntity, "metadata column %s not found in CSV header", col)
		}

		switch dtype {
		case MetadataTypeString:
			metdataParsers[col] = func(value string) (any, error) {
				return value, nil
			}
		case MetadataTypeInt:
			metdataParsers[col] = func(value string) (any, error) {
				return strconv.Atoi(value)
			}
		case MetadataTypeFloat:
			metdataParsers[col] = func(value string) (any, error) {
				return strconv.ParseFloat(value, 32)
			}
		case MetadataTypeBool:
			metdataParsers[col] = func(value string) (any, error) {
				cleaned := strings.ToLower(strings.TrimSpace(value))
				if cleaned == "true" || cleaned == "1" {
					return true, nil
				} else if cleaned == "false" || cleaned == "0" {
					return false, nil
				}
				return nil, fmt.Errorf("invalid boolean value: %s, expected true/1 or false/0", value)
			}
		}
	}
	return metdataParsers, nil
}

func ParseContent(data []byte, textCols []string, metadataTypes map[string]string, docMetadata map[string]any) ([]string, []map[string]any, error) {
	reader := csv.NewReader(bytes.NewReader(data))

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, CodedErrorf(http.StatusUnprocessableEntity, "only CSV files are supported: unable to read CSV header: %w", err)
	}

	if len(rows) < 1 {
		return nil, nil, CodedErrorf(http.StatusUnprocessableEntity, "CSV file is empty")
	}

	header := rows[0]

	colToIdx := make(map[string]int, len(header))
	for i, col := range header {
		colToIdx[col] = i
	}

	for _, col := range textCols {
		if _, ok := colToIdx[col]; !ok {
			return nil, nil, CodedErrorf(http.StatusUnprocessableEntity, "column '%s' specified for indexing is not present in the CSV header", col)
		}
	}

	metdataParsers, err := buildMetadataParsers(colToIdx, metadataTypes)
	if err != nil {
		return nil, nil, err
	}

	chunks := make([]string, 0, len(rows)-1)
	metadata := make([]map[string]any, 0, len(rows)-1)

	for _, row := range rows[1:] {
		chunk := strings.Builder{}
		for _, col := range textCols {
			chunk.WriteString(row[colToIdx[col]])
			chunk.WriteRune(' ')
		}
		chunks = append(chunks, strings.TrimSpace(chunk.String()))

		meta := make(map[string]any, len(metdataParsers)+len(docMetadata))
		for k, v := range docMetadata {
			meta[k] = v
		}
		for col, parser := range metdataParsers {
			value := row[colToIdx[col]]
			parsedValue, err := parser(value)
			if err != nil {
				return nil, nil, CodedErrorf(http.StatusUnprocessableEntity, "error parsing metadata column %s value %s: %w", col, value, err)
			}
			meta[col] = parsedValue
		}
		metadata = append(metadata, meta)
	}

	return chunks, metadata, nil
}
