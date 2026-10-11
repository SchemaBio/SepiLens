package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"
)

func ValidateJSON(raw string, schema map[string]any) error {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return errors.New("invalid JSON")
	}
	return validate(v, schema)
}
func validate(v any, s map[string]any) error {
	if enums, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enums {
			found = found || reflect.DeepEqual(e, v)
		}
		if !found {
			return errors.New("invalid enum")
		}
	}
	switch s["type"] {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("object required")
		}
		props, _ := s["properties"].(map[string]any)
		required, _ := s["required"].([]any)
		for _, k := range required {
			if _, ok := m[fmt.Sprint(k)]; !ok {
				return errors.New("required property missing")
			}
		}
		for k, x := range m {
			p, ok := props[k]
			if !ok {
				if s["additionalProperties"] == false {
					return errors.New("unexpected property")
				}
				continue
			}
			if ps, ok := p.(map[string]any); ok {
				if err := validate(x, ps); err != nil {
					return err
				}
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return errors.New("array required")
		}
		p, _ := s["items"].(map[string]any)
		if max, ok := s["maxItems"].(float64); ok && len(a) > int(max) {
			return errors.New("too many items")
		}
		if min, ok := s["minItems"].(float64); ok && len(a) < int(min) {
			return errors.New("too few items")
		}
		for _, x := range a {
			if err := validate(x, p); err != nil {
				return err
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return errors.New("string required")
		}
		if max, ok := s["maxLength"].(float64); ok && utf8.RuneCountInString(str) > int(max) {
			return errors.New("string too long")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return errors.New("boolean required")
		}
	case "number":
		if _, ok := v.(float64); !ok {
			return errors.New("number required")
		}
	case "integer":
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			return errors.New("integer required")
		}
	}
	return nil
}
