package translator

import "fmt"

// cleanJSONSchemaForGemini sanitizes a JSON Schema for Gemini/Antigravity API compatibility.
// Gemini rejects many standard JSON Schema keywords (additionalProperties, $schema, format, etc.)
// and requires explicit type annotations. This function recursively normalizes the schema.
// Ported from 9router open-sse/translator/formats/gemini.js cleanJSONSchemaForAntigravity.
//
// All walkers thread an isSchema flag to distinguish schema nodes from property-name
// maps (9router#2884). A JSON Schema tree alternates:
//
//	schema node → (key "properties") → property-name map → (any key) → schema node
//
// Keys in a name map are user-defined parameter names and must never be treated as
// schema keywords or have type annotations injected.
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

// childSchema computes the isSchema flag for a child map value.
// From a schema node, the "properties" key leads to a name map (not a schema).
// From a name map, all children are schema nodes again.
func childSchema(isSchema bool, key string) bool {
	return !isSchema || key != "properties"
}

// walkChildren applies fn to nested map/slice children with correct isSchema propagation.
func walkChildren(obj map[string]any, isSchema bool, fn func(map[string]any, bool)) {
	for key, val := range obj {
		switch v := val.(type) {
		case map[string]any:
			fn(v, childSchema(isSchema, key))
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					fn(m, isSchema)
				}
			}
		}
	}
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
	removeUnsupportedKeywordsWalk(obj, true)
}

func removeUnsupportedKeywordsWalk(obj map[string]any, isSchema bool) {
	for key, val := range obj {
		if isSchema && (unsupportedSchemaKeywords[key] || (len(key) > 2 && key[:2] == "x-")) {
			delete(obj, key)
			continue
		}
		switch v := val.(type) {
		case map[string]any:
			removeUnsupportedKeywordsWalk(v, childSchema(isSchema, key))
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					removeUnsupportedKeywordsWalk(m, isSchema)
				}
			}
		}
	}
}

func convertConstToEnum(obj map[string]any) {
	convertConstToEnumWalk(obj, true)
}

func convertConstToEnumWalk(obj map[string]any, isSchema bool) {
	if isSchema {
		if constVal, ok := obj["const"]; ok {
			if _, hasEnum := obj["enum"]; !hasEnum {
				obj["enum"] = []any{constVal}
			}
			delete(obj, "const")
		}
	}
	walkChildren(obj, isSchema, convertConstToEnumWalk)
}

func convertEnumValuesToStrings(obj map[string]any) {
	convertEnumValuesToStringsWalk(obj, true)
}

func convertEnumValuesToStringsWalk(obj map[string]any, isSchema bool) {
	if isSchema {
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
	}
	walkChildren(obj, isSchema, convertEnumValuesToStringsWalk)
}

func mergeAllOf(obj map[string]any) {
	mergeAllOfWalk(obj, true)
}

func mergeAllOfWalk(obj map[string]any, isSchema bool) {
	if isSchema {
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
	}
	walkChildren(obj, isSchema, mergeAllOfWalk)
}

func flattenAnyOfOneOf(obj map[string]any) {
	flattenAnyOfOneOfWalk(obj, true)
}

func flattenAnyOfOneOfWalk(obj map[string]any, isSchema bool) {
	if isSchema {
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
	}
	walkChildren(obj, isSchema, flattenAnyOfOneOfWalk)
}

func flattenTypeArrays(obj map[string]any) {
	flattenTypeArraysWalk(obj, true)
}

func flattenTypeArraysWalk(obj map[string]any, isSchema bool) {
	if isSchema {
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
	}
	walkChildren(obj, isSchema, flattenTypeArraysWalk)
}

func ensureObjectType(obj map[string]any) {
	ensureObjectTypeWalk(obj, true)
}

func ensureObjectTypeWalk(obj map[string]any, isSchema bool) {
	if isSchema {
		if _, hasProps := obj["properties"]; hasProps {
			if _, hasType := obj["type"]; !hasType {
				obj["type"] = "object"
			}
		}
	}
	walkChildren(obj, isSchema, ensureObjectTypeWalk)
}

func cleanupRequired(obj map[string]any) {
	cleanupRequiredWalk(obj, true)
}

func cleanupRequiredWalk(obj map[string]any, isSchema bool) {
	if isSchema {
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
	}
	walkChildren(obj, isSchema, cleanupRequiredWalk)
}

func addPlaceholders(obj map[string]any) {
	addPlaceholdersWalk(obj, true)
}

func addPlaceholdersWalk(obj map[string]any, isSchema bool) {
	if isSchema {
		// Empty schema {} after $ref removal — promote to object with placeholder (9router@e3e3e23)
		if len(obj) == 0 {
			obj["type"] = "object"
			obj["properties"] = map[string]any{
				"reason": map[string]any{
					"type":        "string",
					"description": "Brief explanation of why you are calling this tool",
				},
			}
			obj["required"] = []any{"reason"}
			return
		}
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
	}
	walkChildren(obj, isSchema, addPlaceholdersWalk)
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

func containsAny(slice []any, item any) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
