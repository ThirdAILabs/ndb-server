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

const (
	MetadataTypeString = "string"
	MetadataTypeInt    = "integer"
	MetadataTypeFloat  = "float"
	MetadataTypeBool   = "boolean"
)

type Constraint struct {
	ConstraintType string `json:"constraint_type"`
	Value          any    `json:"value"`
	Dtype          string `json:"dtype"`
}

func asSliceAny[T any](slice []T) []any {
	result := make([]any, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

func (c *Constraint) UnmarshalJSON(data []byte) error {
	var raw struct {
		ConstraintType string          `json:"constraint_type"`
		Value          json.RawMessage `json:"value"`
		Dtype          string          `json:"dtype"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal constraint value: %w", err)
	}

	c.ConstraintType = raw.ConstraintType
	c.Dtype = raw.Dtype

	if raw.ConstraintType == AnyOfType {
		switch raw.Dtype {
		case MetadataTypeString:
			var value []string
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal string value: %w", err)
			}
			c.Value = asSliceAny(value)
		case MetadataTypeInt:
			var value []int
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal int value: %w", err)
			}
			c.Value = asSliceAny(value)
		case MetadataTypeFloat:
			var value []float64
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal float value: %w", err)
			}
			c.Value = asSliceAny(value)
		case MetadataTypeBool:
			var value []bool
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal bool value: %w", err)
			}
			c.Value = asSliceAny(value)
		default:
			return fmt.Errorf("unknown dtype: %s", raw.Dtype)
		}
	} else {
		switch raw.Dtype {
		case MetadataTypeString:
			var value string
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal string value: %w", err)
			}
			c.Value = value
		case MetadataTypeInt:
			var value int
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal int value: %w", err)
			}
			c.Value = value
		case MetadataTypeFloat:
			var value float64
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal float value: %w", err)
			}
			c.Value = value
		case MetadataTypeBool:
			var value bool
			if err := json.Unmarshal(raw.Value, &value); err != nil {
				return fmt.Errorf("failed to unmarshal bool value: %w", err)
			}
			c.Value = value
		default:
			return fmt.Errorf("unknown dtype: %s", raw.Dtype)
		}
	}

	return nil
}

func (c *Constraint) asNDBConstraint() (ndb.Constraint, error) {
	switch c.ConstraintType {
	case AnyOfType:
		if values, ok := c.Value.([]any); ok {
			return ndb.AnyOf(values), nil
		}
		return nil, fmt.Errorf("missing or invalid 'value' argument for AnyOf constraint")
	case EqualToType:
		if c.Value != nil {
			return ndb.EqualTo(c.Value), nil
		}
		return nil, fmt.Errorf("missing or invalid 'value' argument for EqualTo constraint")
	case SubstringType:
		if c.Value != nil {
			return ndb.Substring(c.Value), nil
		}
		return nil, fmt.Errorf("missing or invalid 'value' argument for Substring constraint")
	case GreaterThanType:
		if c.Value != nil {
			return ndb.GreaterThan(c.Value), nil
		}
		return nil, fmt.Errorf("missing or invalid 'value' argument for GreaterThan constraint")
	case LessThanType:
		if c.Value != nil {
			return ndb.LessThan(c.Value), nil
		}
		return nil, fmt.Errorf("missing or invalid 'value' argument for LessThan constraint")
	default:
		return nil, fmt.Errorf("unknown constraint_type: %s", c.ConstraintType)
	}
}

type NDBSearchParams struct {
	Query       string                `json:"query"`
	TopK        int                   `json:"top_k"`
	Constraints map[string]Constraint `json:"constraints"`
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
	Query      string      `json:"query_text"`
	References []Reference `json:"references"`
}

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
