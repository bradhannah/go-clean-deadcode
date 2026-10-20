// Package contract publishes JSON schemas directly from the versioned wire types.
package contract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
)

func Schema(kind string) ([]byte, error) {
	var typ reflect.Type
	switch kind {
	case "report":
		typ = reflect.TypeFor[report.Report]()
	case "config":
		typ = reflect.TypeFor[config.Config]()
	default:
		return nil, fmt.Errorf("schema kind must be report or config")
	}
	defs := map[string]any{}
	var describe func(reflect.Type) map[string]any
	describe = func(t reflect.Type) map[string]any {
		switch t.Kind() {
		case reflect.Pointer:
			return map[string]any{"anyOf": []any{describe(t.Elem()), map[string]any{"type": "null"}}}
		case reflect.Struct:
			name := t.Name()
			if _, ok := defs[name]; !ok {
				defs[name] = nil
				props := map[string]any{}
				required := []string{}
				for i := range t.NumField() {
					f := t.Field(i)
					parts := strings.Split(f.Tag.Get("json"), ",")
					if parts[0] == "-" {
						continue
					}
					key := parts[0]
					if key == "" {
						key = f.Name
					}
					props[key] = describe(f.Type)
					if len(parts) == 1 {
						required = append(required, key)
					}
				}
				if name == "Config" || name == "Report" {
					props["schema_version"] = map[string]any{"const": 1}
				}
				defs[name] = map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
			}
			return map[string]any{"$ref": "#/$defs/" + name}
		case reflect.Slice:
			return map[string]any{"type": []string{"array", "null"}, "items": describe(t.Elem())}
		case reflect.String:
			return map[string]any{"type": "string"}
		case reflect.Bool:
			return map[string]any{"type": "boolean"}
		case reflect.Int, reflect.Int64:
			return map[string]any{"type": "integer"}
		default:
			panic("unsupported wire type: " + t.String())
		}
	}
	root := describe(typ)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = "https://github.com/bradhannah/go-clean-deadcode/schemas/" + kind + "-v1.schema.json"
	root["$defs"] = defs
	b, err := json.MarshalIndent(root, "", "  ")
	return append(b, '\n'), err
}
