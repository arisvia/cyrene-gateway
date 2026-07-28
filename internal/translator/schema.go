package translator

import "fmt"

// cleanJSONSchemaForGemini sanitizes a JSON Schema for Gemini/Antigravity API compatibility.
// Gemini rejects many standard JSON Schema keywords (additionalProperties, $schema, format, etc.)
// and requires explicit type annotations. This function recursively normalizes the schema.
// Ported from 9router open-sse/translator/formats/gemini.js cleanJSONSchemaForAntigravity.
func cleanJSONSchemaForGemini(schema map[string]any) map[string]any {
	if schema == nil {
		return schema
	}

	// Phase 1: Convert and prepare
	convertConstToEnum(schema)
	convertEnumValuesToStrings(schema)

	// Phase 2: Flatten complex structures
	mergeAllOf(schema)
	flattenAnyOfOneOf(schema)
	flattenTypeArrays(schema)

	// Phase 2.5: Infer missing type=object when properties exist
	ensureObjectType(schema)

	// Phase 3: Remove unsupported keywords
	removeUnsupportedKeywords(schema)

	// Phase 4: Cleanup required fields
	cleanupRequired(schema)

	// Phase 5: Add placeholder for empty object schemas
	addPlaceholders(schema)

	return schema
}

// unsupportedSchemaKeywords are JSON Schema keywords rejected by Gemini/Antigravity APIs.
var unsupportedSchemaKeywords = map[string]bool{
	// Basic constraints
	"minLength": true, "maxLength": true, "exclusiveMinimum": true, "exclusiveMaximum": true,
	"minItems": true, "maxItems": true, "format": true,
	// Claude VALIDATED mode rejects these
	"default": true, "examples": true,
	// JSON Schema meta keywords
	"$schema": true, "$defs": true, "definitions": true, "const": true, "$ref": true, "$comment": true,
	// Annotation keywords
	"deprecated": true, "readOnly": true, "writeOnly": true,
	// Object validation keywords
	"additionalProperties": true, "propertyNames": true, "patternProperties": true, "enumDescriptions": true,
	// Complex schema keywords (handled by flatten functions)
	"anyOf": true, "oneOf": true, "allOf": true, "not": true,
	// Dependency keywords
	"dependencies": true, "dependentSchemas": true, "dependentRequired": true,
	// Other unsupported
	"title": true, "optional": true, "if": true, "then": true, "else": true,
	"contentMediaType": true, "contentEncoding": true,
	// UI/Styling properties (from Cursor tools)
	"cornerRadius": true, "fillColor": true, "fontFamily": true, "fontSize": true,
	"fontWeight": true, "gap": true, "padding": true, "strokeColor": true,
	"strokeThickness": true, "textColor": true,
}

func removeUnsupportedKeywords(obj map[string]any) {
	for key, val := range obj {
		if unsupportedSchemaKeywords[key] || (len(key) > 2 && key[:2] == "x-") {
			delete(obj, key)
			continue
		}
		recurseSchema(val, removeUnsupportedKeywords)
	}
}

func convertConstToEnum(obj map[string]any) {
	if constVal, ok := obj["const"]; ok {
		if _, hasEnum := obj["enum"]; !hasEnum {
			obj["enum"] = []any{constVal}
		}
		delete(obj, "const")
	}
	for _, val := range obj {
		recurseSchema(val, convertConstToEnum)
	}
}

func convertEnumValuesToStrings(obj map[string]any) {
	if enumRaw, ok := obj["enum"]; ok {
		if enumArr, ok := enumRaw.([]any); ok {
			strs := make([]any, len(enumArr))
			for i, v := range enumArr {
				strs[i] = fmt.Sprintf("%v", v)
			}
			obj["enum"] = strs
			// Gemini requires type:"string" when enum is present
			if _, hasType := obj["type"]; !hasType {
				obj["type"] = "string"
			}
		}
	}
	for _, val := range obj {
		recurseSchema(val, convertEnumValuesToStrings)
	}
}

func mergeAllOf(obj map[string]any) {
	if allOfRaw, ok := obj["allOf"]; ok {
		if allOf, ok := allOfRaw.([]any); ok {
			mergedProps := map[string]any{}
			var mergedRequired []any

			for _, itemRaw := range allOf {
				item, ok := itemRaw.(map[string]any)
				if !ok {
					continue
				}
				if props, ok := item["properties"].(map[string]any); ok {
					for k, v := range props {
						mergedProps[k] = v
					}
				}
				if req, ok := item["required"].([]any); ok {
					for _, r := range req {
						if !containsAny(mergedRequired, r) {
							mergedRequired = append(mergedRequired, r)
						}
					}
				}
			}

			delete(obj, "allOf")
			if len(mergedProps) > 0 {
				existing, _ := obj["properties"].(map[string]any)
				if existing == nil {
					existing = map[string]any{}
				}
				for k, v := range mergedProps {
					existing[k] = v
				}
				obj["properties"] = existing
			}
			if len(mergedRequired) > 0 {
				existingReq, _ := obj["required"].([]any)
				obj["required"] = append(existingReq, mergedRequired...)
			}
		}
	}
	for _, val := range obj {
		recurseSchema(val, mergeAllOf)
	}
}

func flattenAnyOfOneOf(obj map[string]any) {
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if raw, ok := obj[keyword]; ok {
			if arr, ok := raw.([]any); ok && len(arr) > 0 {
				var nonNull []map[string]any
				for _, itemRaw := range arr {
					item, ok := itemRaw.(map[string]any)
					if !ok {
						continue
					}
					if t, _ := item["type"].(string); t != "null" {
						nonNull = append(nonNull, item)
					}
				}
				if len(nonNull) > 0 {
					selected := nonNull[selectBestSchema(nonNull)]
					delete(obj, keyword)
					for k, v := range selected {
						obj[k] = v
					}
				}
			}
		}
	}
	for _, val := range obj {
		recurseSchema(val, flattenAnyOfOneOf)
	}
}

func flattenTypeArrays(obj map[string]any) {
	if typeRaw, ok := obj["type"]; ok {
		if typeArr, ok := typeRaw.([]any); ok {
			var nonNull []string
			for _, t := range typeArr {
				if s, ok := t.(string); ok && s != "null" {
					nonNull = append(nonNull, s)
				}
			}
			if len(nonNull) > 0 {
				obj["type"] = nonNull[0]
			} else {
				obj["type"] = "string"
			}
		}
	}
	for _, val := range obj {
		recurseSchema(val, flattenTypeArrays)
	}
}

func ensureObjectType(obj map[string]any) {
	if _, hasProps := obj["properties"]; hasProps {
		if _, hasType := obj["type"]; !hasType {
			obj["type"] = "object"
		}
	}
	for _, val := range obj {
		recurseSchema(val, ensureObjectType)
	}
}

func cleanupRequired(obj map[string]any) {
	if reqRaw, ok := obj["required"]; ok {
		if reqArr, ok := reqRaw.([]any); ok {
			if props, ok := obj["properties"].(map[string]any); ok {
				var valid []any
				for _, r := range reqArr {
					if rStr, ok := r.(string); ok {
						if _, exists := props[rStr]; exists {
							valid = append(valid, r)
						}
					}
				}
				if len(valid) == 0 {
					delete(obj, "required")
				} else {
					obj["required"] = valid
				}
			}
		}
	}
	for _, val := range obj {
		recurseSchema(val, cleanupRequired)
	}
}

func addPlaceholders(obj map[string]any) {
	if t, _ := obj["type"].(string); t == "object" {
		props, _ := obj["properties"].(map[string]any)
		if len(props) == 0 {
			obj["properties"] = map[string]any{
				"reason": map[string]any{
					"type":        "string",
					"description": "Brief explanation of why you are calling this tool",
				},
			}
			obj["required"] = []any{"reason"}
		}
	}
	for _, val := range obj {
		recurseSchema(val, addPlaceholders)
	}
}

// selectBestSchema picks the most informative schema from anyOf/oneOf candidates.
func selectBestSchema(items []map[string]any) int {
	bestIdx := 0
	bestScore := -1
	for i, item := range items {
		score := 0
		t, _ := item["type"].(string)
		if t == "object" || item["properties"] != nil {
			score = 3
		} else if t == "array" || item["items"] != nil {
			score = 2
		} else if t != "" && t != "null" {
			score = 1
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return bestIdx
}

// recurseSchema applies fn to nested map[string]any values (properties, items, etc.).
func recurseSchema(val any, fn func(map[string]any)) {
	switch v := val.(type) {
	case map[string]any:
		fn(v)
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				fn(m)
			}
		}
	}
}

func containsAny(slice []any, item any) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
