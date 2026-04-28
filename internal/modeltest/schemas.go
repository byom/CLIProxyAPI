package modeltest

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TravelItinerarySchemaJSON is the canonical complex JSON schema used by the
// structured-output suites. It intentionally nests objects, enums, arrays
// with min/max length, `$ref` definitions and `additionalProperties: false`
// so translator bugs show up quickly.
const TravelItinerarySchemaJSON = `{
    "type": "object",
    "additionalProperties": false,
    "required": ["trip_id", "traveler", "legs", "summary"],
    "properties": {
        "trip_id": {"type": "string", "minLength": 1},
        "traveler": {
            "type": "object",
            "additionalProperties": false,
            "required": ["name", "loyalty_tier"],
            "properties": {
                "name": {"type": "string", "minLength": 1},
                "loyalty_tier": {
                    "type": "string",
                    "enum": ["bronze", "silver", "gold", "platinum"]
                },
                "preferences": {
                    "type": "object",
                    "additionalProperties": false,
                    "properties": {
                        "seat": {
                            "type": "string",
                            "enum": ["window", "aisle", "middle", "any"]
                        },
                        "diet": {"type": "string"}
                    }
                }
            }
        },
        "legs": {
            "type": "array",
            "minItems": 1,
            "maxItems": 5,
            "items": {
                "type": "object",
                "additionalProperties": false,
                "required": [
                    "sequence",
                    "mode",
                    "origin",
                    "destination",
                    "departure_local",
                    "duration_minutes"
                ],
                "properties": {
                    "sequence": {"type": "integer", "minimum": 1},
                    "mode": {
                        "type": "string",
                        "enum": ["flight", "train", "bus", "car", "ferry"]
                    },
                    "origin": {"$ref": "#/$defs/place"},
                    "destination": {"$ref": "#/$defs/place"},
                    "departure_local": {"type": "string", "minLength": 1},
                    "duration_minutes": {"type": "integer", "minimum": 1},
                    "carrier": {"type": "string"},
                    "booking_reference": {"type": "string"}
                }
            }
        },
        "alerts": {
            "type": "array",
            "items": {
                "type": "object",
                "additionalProperties": false,
                "required": ["severity", "message"],
                "properties": {
                    "severity": {
                        "type": "string",
                        "enum": ["info", "warning", "critical"]
                    },
                    "message": {"type": "string", "minLength": 1}
                }
            }
        },
        "summary": {"type": "string", "minLength": 1}
    },
    "$defs": {
        "place": {
            "type": "object",
            "additionalProperties": false,
            "required": ["city", "country", "coordinates"],
            "properties": {
                "city": {"type": "string", "minLength": 1},
                "country": {"type": "string", "minLength": 1},
                "coordinates": {
                    "type": "object",
                    "additionalProperties": false,
                    "required": ["latitude", "longitude"],
                    "properties": {
                        "latitude": {"type": "number", "minimum": -90, "maximum": 90},
                        "longitude": {"type": "number", "minimum": -180, "maximum": 180}
                    }
                }
            }
        }
    }
}`

// TravelItinerarySchema returns a decoded map of the canonical schema. A
// fresh copy is returned on each call so callers may mutate it freely.
func TravelItinerarySchema() map[string]interface{} {
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(TravelItinerarySchemaJSON), &out); err != nil {
		// Constant literal is trusted; panic here would only surface a test bug.
		panic(fmt.Sprintf("modeltest: invalid TravelItinerarySchema constant: %v", err))
	}
	return out
}

// TravelItinerarySchemaForGemini returns the schema with `$ref` inlined and
// keys unsupported by Gemini's responseSchema stripped.
func TravelItinerarySchemaForGemini() map[string]interface{} {
	schema := TravelItinerarySchema()
	inlined := inlineRefs(schema)
	stripped := stripKeys(inlined, map[string]struct{}{
		"additionalProperties": {},
		"$schema":              {},
		"$id":                  {},
	})
	return stripped.(map[string]interface{})
}

// TravelItinerarySchemaForOpenAIStrict returns a schema compatible with
// OpenAI's `strict: true` structured-output mode. The only rule
// OpenAI rejects in our canonical schema is that every key defined in
// `properties` must also appear in `required`; this helper walks the schema
// and injects the missing required entries at every object level. Other
// constraint keywords (minItems, minimum, maxLength, ...) are preserved so
// the server's own validation still fires, and the canonical schema keeps
// its stricter local constraints unchanged.
func TravelItinerarySchemaForOpenAIStrict() map[string]interface{} {
	schema := TravelItinerarySchema()
	return fillRequiredEverywhere(schema).(map[string]interface{})
}

// TravelItineraryPrompt returns the deterministic user prompt shared across
// the structured-output suites.
func TravelItineraryPrompt() string {
	return "Plan a three-leg international trip starting in Tokyo. " +
		"Leg 1: Tokyo (Japan) to Seoul (South Korea) by flight, " +
		"leg 2: Seoul to Osaka (Japan) by ferry, " +
		"leg 3: Osaka to Kyoto (Japan) by train. " +
		"The traveler is named Alex Rivera, gold loyalty tier, " +
		"prefers window seats and a vegetarian diet. " +
		"Include at least one informational alert about local weather. " +
		"Return ONLY the structured travel itinerary; do not include any prose."
}

// ValidateTravelItinerary validates a decoded JSON payload against the
// canonical schema. It returns an empty string when the payload is valid.
func ValidateTravelItinerary(payload interface{}) string {
	compiled, err := compileTravelItinerarySchema()
	if err != nil {
		return fmt.Sprintf("schema compile error: %v", err)
	}
	if err := compiled.Validate(payload); err != nil {
		return err.Error()
	}
	return ""
}

func compileTravelItinerarySchema() (*jsonschema.Schema, error) {
	var raw any
	if err := json.Unmarshal([]byte(TravelItinerarySchemaJSON), &raw); err != nil {
		return nil, fmt.Errorf("decode schema json: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("travel_itinerary.json", raw); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	return c.Compile("travel_itinerary.json")
}

// inlineRefs walks the schema tree and replaces every {"$ref":"#/$defs/x"}
// with a deep copy of $defs.x. The top-level $defs key is removed at the end.
func inlineRefs(schema map[string]interface{}) interface{} {
	defs, _ := schema["$defs"].(map[string]interface{})

	var resolve func(node interface{}) interface{}
	resolve = func(node interface{}) interface{} {
		switch v := node.(type) {
		case map[string]interface{}:
			if ref, ok := v["$ref"].(string); ok && len(ref) > 8 && ref[:8] == "#/$defs/" {
				name := ref[8:]
				target, ok := defs[name]
				if !ok {
					return v
				}
				return resolve(deepCopy(target))
			}
			out := make(map[string]interface{}, len(v))
			for k, val := range v {
				if k == "$defs" {
					continue
				}
				out[k] = resolve(val)
			}
			return out
		case []interface{}:
			out := make([]interface{}, len(v))
			for i, item := range v {
				out[i] = resolve(item)
			}
			return out
		default:
			return v
		}
	}

	copy := deepCopy(schema).(map[string]interface{})
	delete(copy, "$defs")
	return resolve(copy)
}

func stripKeys(node interface{}, drop map[string]struct{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, val := range v {
			if _, skip := drop[k]; skip {
				continue
			}
			out[k] = stripKeys(val, drop)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = stripKeys(item, drop)
		}
		return out
	default:
		return v
	}
}

// fillRequiredEverywhere walks the schema tree and, for every object schema
// that has a `properties` map, ensures its `required` array lists all defined
// property keys (alphabetised for a stable output).
func fillRequiredEverywhere(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, val := range v {
			out[k] = fillRequiredEverywhere(val)
		}
		if props, ok := out["properties"].(map[string]interface{}); ok {
			keys := make([]string, 0, len(props))
			for k := range props {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			required := make([]interface{}, 0, len(keys))
			for _, k := range keys {
				required = append(required, k)
			}
			out["required"] = required
		}
		return out
	case []interface{}:
		copy := make([]interface{}, len(v))
		for i, item := range v {
			copy[i] = fillRequiredEverywhere(item)
		}
		return copy
	default:
		return v
	}
}

func deepCopy(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, val := range v {
			out[k] = deepCopy(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = deepCopy(item)
		}
		return out
	default:
		return v
	}
}
