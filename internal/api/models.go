package api

import (
	"encoding/json"
	"fmt"
	"ndb-server/internal/ndb"
)

const (
	AnyOfType       = "AnyOf"
	EqualToType     = "EqualTo"
	SubstringType   = "Substring"
	InRangeType     = "InRange"
	GreaterThanType = "GreaterThan"
	LessThanType    = "LessThan"
)

func parseConstraint(raw map[string]interface{}) (ndb.Constraint, error) {
	if constraintType, ok := raw["constraint_type"].(string); ok {
		switch constraintType {
		case AnyOfType:
			if values, ok := raw["values"].([]interface{}); ok {
				return ndb.AnyOf(values), nil
			}
			return nil, fmt.Errorf("missing or invalid 'values' argument for AnyOf constraint")
		case EqualToType:
			if value, ok := raw["value"]; ok {
				return ndb.EqualTo(value), nil
			}
			return nil, fmt.Errorf("missing or invalid 'value' argument for EqualTo constraint")
		case SubstringType:
			if value, ok := raw["value"].(string); ok {
				return ndb.Substring(value), nil
			}
			return nil, fmt.Errorf("missing or invalid 'value' argument for Substring constraint")
		case GreaterThanType:
			if minimum, ok := raw["minimum"]; ok {
				return ndb.GreaterThan(minimum), nil
			}
			return nil, fmt.Errorf("missing or invalid 'minimum' argument for GreaterThan constraint")
		case LessThanType:
			if maximum, ok := raw["maximum"]; ok {
				return ndb.LessThan(maximum), nil
			}
			return nil, fmt.Errorf("missing or invalid 'maximum' argument for LessThan constraint")
		default:
			return nil, fmt.Errorf("unknown constraint_type: %s", constraintType)
		}
	}

	return nil, fmt.Errorf("invalid or missing constraint_type field")
}

type Constraints ndb.Constraints

func (c *Constraints) UnmarshalJSON(data []byte) error {
	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	constraints := make(ndb.Constraints)

	for key, constraint := range raw {
		constraint, err := parseConstraint(constraint)
		if err != nil {
			return fmt.Errorf("failed to parse constraint for key %s: %w", key, err)
		}
		constraints[key] = constraint
	}

	*c = constraints

	return nil
}

type NDBSearchParams struct {
	Query       string      `json:"query"`
	TopK        int         `json:"top_k"`
	Constraints Constraints `json:"constraints"`
}

type Reference struct {
	Id       uint64                 `json:"id"`
	Text     string                 `json:"text"`
	Source   string                 `json:"source"`
	SourceId string                 `json:"source_id"`
	Metadata map[string]interface{} `json:"metadata"`
	Score    float32                `json:"score"`
}

type NDBSearchResponse struct {
	Query     string      `json:"query_text"`
	Reference []Reference `json:"references"`
}

const (
	MetadataTypeString = "string"
	MetadataTypeInt    = "integer"
	MetadataTypeFloat  = "float"
	MetadataTypeBool   = "boolean"
)

type NDBDocumentMetadata struct {
	Filename      string            `json:"filename"`
	SourceId      *string           `json:"source_id"`
	TextColumns   []string          `json:"text_columns"`
	MetadataTypes map[string]string `json:"metadata_types"`
	DocMetadata   map[string]any    `json:"metadata"`
}

type NDBDeleteParams struct {
	SourceIds []string `json:"source_ids"`
}

type NDBUpvoteParams struct {
	TextIdPairs []struct {
		QueryText   string `json:"query_text"`
		ReferenceId uint64 `json:"reference_id"`
	} `json:"text_id_pairs"`
}

type NDBSource struct {
	Source   string `json:"source"`
	SourceId string `json:"source_id"`
	Version  uint32 `json:"version"`
}

type NDBCheckpointResponse struct {
	Version       int  `json:"version"`
	NewCheckpoint bool `json:"new_checkpoint"`
}
