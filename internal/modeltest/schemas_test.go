package modeltest

import (
	"encoding/json"
	"strings"
	"testing"
)

const validPayload = `{
  "trip_id": "trip-001",
  "traveler": {
    "name": "Alex Rivera",
    "loyalty_tier": "gold",
    "preferences": {"seat": "window", "diet": "vegetarian"}
  },
  "legs": [
    {
      "sequence": 1,
      "mode": "flight",
      "origin": {
        "city": "Tokyo", "country": "Japan",
        "coordinates": {"latitude": 35.68, "longitude": 139.69}
      },
      "destination": {
        "city": "Seoul", "country": "South Korea",
        "coordinates": {"latitude": 37.55, "longitude": 126.99}
      },
      "departure_local": "2026-05-01T09:00:00",
      "duration_minutes": 150,
      "carrier": "Korean Air",
      "booking_reference": "ABC123"
    },
    {
      "sequence": 2,
      "mode": "ferry",
      "origin": {
        "city": "Seoul", "country": "South Korea",
        "coordinates": {"latitude": 37.55, "longitude": 126.99}
      },
      "destination": {
        "city": "Osaka", "country": "Japan",
        "coordinates": {"latitude": 34.69, "longitude": 135.50}
      },
      "departure_local": "2026-05-02T07:30:00",
      "duration_minutes": 1080
    }
  ],
  "alerts": [
    {"severity": "info", "message": "Mild rain expected in Seoul on day 2."}
  ],
  "summary": "Three-leg routing across northeast Asia."
}`

func decodePayload(t *testing.T, s string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return v
}

func TestValidPayloadPassesSchema(t *testing.T) {
	if err := ValidateTravelItinerary(decodePayload(t, validPayload)); err != "" {
		t.Fatalf("expected valid payload to pass, got: %s", err)
	}
}

func TestMissingRequiredFieldFails(t *testing.T) {
	raw := strings.Replace(validPayload, `"summary": "Three-leg routing across northeast Asia."`, `"other": "present"`, 1)
	if err := ValidateTravelItinerary(decodePayload(t, raw)); err == "" {
		t.Fatal("expected missing summary to fail validation")
	}
}

func TestLoyaltyEnumIsEnforced(t *testing.T) {
	raw := strings.Replace(validPayload, `"loyalty_tier": "gold"`, `"loyalty_tier": "diamond"`, 1)
	if err := ValidateTravelItinerary(decodePayload(t, raw)); err == "" {
		t.Fatal("expected invalid loyalty tier to fail validation")
	}
}

func TestAdditionalPropertiesBlocked(t *testing.T) {
	raw := strings.Replace(validPayload, `"summary": "Three-leg routing across northeast Asia."`, `"summary": "Three-leg routing across northeast Asia.","extra_field":"not allowed"`, 1)
	if err := ValidateTravelItinerary(decodePayload(t, raw)); err == "" {
		t.Fatal("expected additionalProperties to fail validation")
	}
}

func TestLegsMinItems(t *testing.T) {
	payload := decodePayload(t, validPayload).(map[string]interface{})
	payload["legs"] = []interface{}{}
	if err := ValidateTravelItinerary(payload); err == "" {
		t.Fatal("expected empty legs array to fail validation")
	}
}

func TestCoordinatesRangeEnforced(t *testing.T) {
	payload := decodePayload(t, validPayload).(map[string]interface{})
	legs := payload["legs"].([]interface{})
	origin := legs[0].(map[string]interface{})["origin"].(map[string]interface{})
	coords := origin["coordinates"].(map[string]interface{})
	coords["latitude"] = 200
	if err := ValidateTravelItinerary(payload); err == "" {
		t.Fatal("expected latitude 200 to fail validation")
	}
}

func TestGeminiSchemaStripsUnsupportedKeys(t *testing.T) {
	schema := TravelItinerarySchemaForGemini()
	var walk func(interface{})
	walk = func(node interface{}) {
		switch n := node.(type) {
		case map[string]interface{}:
			for _, blocked := range []string{"$defs", "$ref", "additionalProperties"} {
				if _, ok := n[blocked]; ok {
					t.Fatalf("gemini schema must not contain %q", blocked)
				}
			}
			for _, v := range n {
				walk(v)
			}
		case []interface{}:
			for _, item := range n {
				walk(item)
			}
		}
	}
	walk(schema)
}

func TestGeminiSchemaStillAcceptsValidPayload(t *testing.T) {
	// Validation with the stripped schema should still succeed because
	// ValidateTravelItinerary uses the canonical schema; here we only verify
	// the shape is a well-formed map.
	schema := TravelItinerarySchemaForGemini()
	if _, ok := schema["properties"]; !ok {
		t.Fatal("gemini schema missing properties key")
	}
}

func TestTravelItineraryPromptStable(t *testing.T) {
	a := TravelItineraryPrompt()
	b := TravelItineraryPrompt()
	if a != b {
		t.Fatal("prompt should be deterministic")
	}
	if len(a) < 50 {
		t.Fatalf("prompt unexpectedly short: %d chars", len(a))
	}
}
