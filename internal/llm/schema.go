package llm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"unicode/utf8"
)

// SchemaValidationError contains only contract paths and fixed reasons, never model values.
type SchemaValidationError struct{ Path, Reason string }

func (e *SchemaValidationError) Error() string { return e.Path + ": " + e.Reason }

func ValidateJSON(raw string, schema map[string]any) error {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return &SchemaValidationError{"$", "invalid JSON"}
	}
	return validateAt(v, schema, "$")
}
func validate(v any, s map[string]any) error { return validateAt(v, s, "$") }
func schemaLimit(value any) (int, bool) {
	switch n := value.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}
func validateAt(v any, s map[string]any, path string) error {
	// Structured contracts use nullable fields for scores belonging to other
	// engines. Check every union member rather than skipping union validation.
	var types []any
	switch values := s["type"].(type) {
	case []any:
		types = values
	case []string:
		for _, value := range values {
			types = append(types, value)
		}
	}
	if types != nil {
		for _, typ := range types {
			branch := make(map[string]any, len(s))
			for key, value := range s {
				branch[key] = value
			}
			branch["type"] = typ
			if validateAt(v, branch, path) == nil {
				return nil
			}
		}
		return &SchemaValidationError{path, "no permitted type matched"}
	}
	if raw, ok := s["enum"].([]string); ok {
		values := make([]any, len(raw))
		for i, v := range raw {
			values[i] = v
		}
		copy := make(map[string]any, len(s))
		for k, v := range s {
			copy[k] = v
		}
		copy["enum"] = values
		return validateAt(v, copy, path)
	}
	if enums, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enums {
			found = found || reflect.DeepEqual(e, v)
		}
		if !found {
			return &SchemaValidationError{path, "invalid enum"}
		}
	}
	switch s["type"] {
	case "null":
		if v != nil {
			return &SchemaValidationError{path, "null required"}
		}
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return &SchemaValidationError{path, "object required"}
		}
		props, _ := s["properties"].(map[string]any)
		required, _ := s["required"].([]any)
		for _, k := range required {
			if _, ok := m[fmt.Sprint(k)]; !ok {
				return &SchemaValidationError{path + "." + fmt.Sprint(k), "required property missing"}
			}
		}
		for k, x := range m {
			p, ok := props[k]
			if !ok {
				if s["additionalProperties"] == false {
					return &SchemaValidationError{path, "unexpected property"}
				}
				continue
			}
			if ps, ok := p.(map[string]any); ok {
				if err := validateAt(x, ps, path+"."+k); err != nil {
					return err
				}
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return &SchemaValidationError{path, "array required"}
		}
		if s["uniqueItems"] == true {
			seen := map[string]bool{}
			for _, x := range a {
				key, _ := json.Marshal(x)
				if seen[string(key)] {
					return &SchemaValidationError{path, "duplicate item"}
				}
				seen[string(key)] = true
			}
		}
		p, _ := s["items"].(map[string]any)
		if max, ok := schemaLimit(s["maxItems"]); ok && len(a) > int(max) {
			return &SchemaValidationError{path, "too many items"}
		}
		if min, ok := schemaLimit(s["minItems"]); ok && len(a) < int(min) {
			return &SchemaValidationError{path, "too few items"}
		}
		for i, x := range a {
			if err := validateAt(x, p, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return &SchemaValidationError{path, "string required"}
		}
		if max, ok := schemaLimit(s["maxLength"]); ok && utf8.RuneCountInString(str) > int(max) {
			return &SchemaValidationError{path, "string too long"}
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return &SchemaValidationError{path, "boolean required"}
		}
	case "number":
		if _, ok := v.(float64); !ok {
			return &SchemaValidationError{path, "number required"}
		}
	case "integer":
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			return &SchemaValidationError{path, "integer required"}
		}
	}
	return nil
}
