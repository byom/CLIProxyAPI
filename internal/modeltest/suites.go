package modeltest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// SuiteModels exercises GET /v1/models.
func SuiteModels(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "GET /v1/models"
	sw := StartStopwatch()
	reqTrace := &Trace{Method: "GET", URL: "/v1/models"}
	resp, err := sc.Client.Get(ctx, "/v1/models", nil, false)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, nil)}
	}
	respTrace := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	data := gjson.GetBytes(resp.Body, "data")
	if !data.IsArray() || len(data.Array()) == 0 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "data is empty or missing", truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	for _, entry := range data.Array() {
		if entry.Get("id").String() == "" {
			return []CheckResult{attach(Fail(name, sw.Elapsed(), "one or more models missing id", ""), reqTrace, respTrace)}
		}
	}
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("%d models", len(data.Array()))), reqTrace, respTrace)}
}

// SuiteChatPlain exercises /v1/chat/completions with a short deterministic prompt.
func SuiteChatPlain(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/chat/completions (plain)"
	payload := map[string]interface{}{
		"model": sc.OpenAIModel,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Reply with the single word: pong."},
		},
		"max_tokens":  32,
		"temperature": 0.0,
	}
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/chat/completions", payload)
	resp, err := sc.Client.PostJSON(ctx, "/v1/chat/completions", payload, nil, false)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, nil)}
	}
	respTrace := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	content := gjson.GetBytes(resp.Body, "choices.0.message.content").String()
	if strings.TrimSpace(content) == "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "empty assistant content", ""), reqTrace, respTrace)}
	}
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("content=%q", truncate(strings.TrimSpace(content), 32))), reqTrace, respTrace)}
}

// SuiteChatJSON exercises response_format=json_schema with the complex schema.
func SuiteChatJSON(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/chat/completions (response_format=json_schema)"
	payload := map[string]interface{}{
		"model":    sc.OpenAIModel,
		"messages": structuredMessages(),
		"response_format": map[string]interface{}{
			"type": "json_schema",
			"json_schema": map[string]interface{}{
				"name":   "travel_itinerary",
				"schema": TravelItinerarySchemaForOpenAIStrict(),
				"strict": true,
			},
		},
		"max_tokens":  1024,
		"temperature": 0.0,
	}
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/chat/completions", payload)
	resp, err := sc.Client.PostJSON(ctx, "/v1/chat/completions", payload, nil, false)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, nil)}
	}
	respTrace := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	content := gjson.GetBytes(resp.Body, "choices.0.message.content").String()
	if strings.TrimSpace(content) == "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "empty content", truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	parsed, err := parseLoose(content)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("content is not JSON: %v", err), truncate(content, 200)), reqTrace, respTrace)}
	}
	if schemaErr := ValidateTravelItinerary(parsed); schemaErr != "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), schemaErr, ""), reqTrace, respTrace)}
	}
	tripID := gjson.Get(content, "trip_id").String()
	legs := gjson.Get(content, "legs").Array()
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("trip_id=%q legs=%d", tripID, len(legs))), reqTrace, respTrace)}
}

// SuiteChatTools exercises a forced function tool_call with the complex schema as parameters.
func SuiteChatTools(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/chat/completions (forced tool call)"
	payload := map[string]interface{}{
		"model":    sc.OpenAIModel,
		"messages": structuredMessages(),
		"tools": []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "record_travel_itinerary",
					"description": "Persist the structured travel itinerary for the user.",
					"parameters":  TravelItinerarySchemaForOpenAIStrict(),
				},
			},
		},
		"tool_choice": map[string]interface{}{
			"type":     "function",
			"function": map[string]interface{}{"name": "record_travel_itinerary"},
		},
		"max_tokens":  1024,
		"temperature": 0.0,
	}
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/chat/completions", payload)
	resp, err := sc.Client.PostJSON(ctx, "/v1/chat/completions", payload, nil, false)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, nil)}
	}
	respTrace := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	toolCalls := gjson.GetBytes(resp.Body, "choices.0.message.tool_calls").Array()
	if len(toolCalls) == 0 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "message.tool_calls is empty", truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	first := toolCalls[0]
	if fn := first.Get("function.name").String(); fn != "record_travel_itinerary" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("unexpected tool name %q", fn), ""), reqTrace, respTrace)}
	}
	argsRaw := first.Get("function.arguments").String()
	if argsRaw == "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "tool_call arguments are empty", ""), reqTrace, respTrace)}
	}
	parsed, err := parseLoose(argsRaw)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("tool arguments not JSON: %v", err), truncate(argsRaw, 200)), reqTrace, respTrace)}
	}
	if schemaErr := ValidateTravelItinerary(parsed); schemaErr != "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), schemaErr, ""), reqTrace, respTrace)}
	}
	tripID := gjson.Get(argsRaw, "trip_id").String()
	legs := gjson.Get(argsRaw, "legs").Array()
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("tool=record_travel_itinerary trip_id=%q legs=%d", tripID, len(legs))), reqTrace, respTrace)}
}

// SuiteChatStream exercises streaming chat completions.
func SuiteChatStream(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/chat/completions (stream)"
	payload := map[string]interface{}{
		"model": sc.OpenAIModel,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Count from one to five, one number per line. Do not add commentary."},
		},
		"max_tokens":  64,
		"temperature": 0.0,
		"stream":      true,
	}
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/chat/completions", payload)
	var (
		doneSeen    bool
		accumulated strings.Builder
		captured    []SSEEvent
	)
	err := sc.Client.StreamPostJSON(ctx, "/v1/chat/completions", payload, nil, false, func(ev SSEEvent) {
		captured = append(captured, ev)
		if ev.IsDone() {
			doneSeen = true
			return
		}
		if ev.Data == "" {
			return
		}
		delta := gjson.Get(ev.Data, "choices.0.delta.content").String()
		if delta != "" {
			accumulated.WriteString(delta)
		}
	})
	respTrace := sseTraceFromEvents(captured, 200)
	events := len(captured)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, respTrace)}
	}
	if events == 0 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "no SSE events received", ""), reqTrace, respTrace)}
	}
	if !doneSeen {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "missing terminal [DONE] event", fmt.Sprintf("events=%d", events)), reqTrace, respTrace)}
	}
	if strings.TrimSpace(accumulated.String()) == "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "no content delta accumulated", fmt.Sprintf("events=%d", events)), reqTrace, respTrace)}
	}
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("events=%d bytes=%d", events, accumulated.Len())), reqTrace, respTrace)}
}

// SuiteResponsesStructured exercises /v1/responses with text.format=json_schema.
func SuiteResponsesStructured(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/responses (text.format json_schema)"
	payload := map[string]interface{}{
		"model": sc.ResponsesModel,
		"input": []map[string]interface{}{
			{
				"role": "system",
				"content": []map[string]interface{}{
					{
						"type": "input_text",
						"text": "You are a travel-planning assistant. Always reply with the structured itinerary requested by the user. Never include explanations or text outside the structured payload.",
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": TravelItineraryPrompt()},
				},
			},
		},
		"text": map[string]interface{}{
			"format": map[string]interface{}{
				"type":   "json_schema",
				"name":   "travel_itinerary",
				"schema": TravelItinerarySchemaForOpenAIStrict(),
				"strict": true,
			},
		},
		"stream":            false,
		"max_output_tokens": 1024,
		"temperature":       0.0,
	}
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/responses", payload)
	resp, err := sc.Client.PostJSON(ctx, "/v1/responses", payload, nil, false)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, nil)}
	}
	respTrace := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	text := extractResponsesText(resp.Body)
	if strings.TrimSpace(text) == "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "response did not contain text output", truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	parsed, err := parseLoose(text)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("output_text is not JSON: %v", err), truncate(text, 200)), reqTrace, respTrace)}
	}
	if schemaErr := ValidateTravelItinerary(parsed); schemaErr != "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), schemaErr, ""), reqTrace, respTrace)}
	}
	tripID := gjson.Get(text, "trip_id").String()
	legs := gjson.Get(text, "legs").Array()
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("trip_id=%q legs=%d", tripID, len(legs))), reqTrace, respTrace)}
}

// SuiteResponsesStream exercises streaming /v1/responses.
func SuiteResponsesStream(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/responses (stream)"
	payload := map[string]interface{}{
		"model": sc.ResponsesModel,
		"input": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": "List three popular Asian cities, one per line."},
				},
			},
		},
		"stream":            true,
		"max_output_tokens": 96,
		"temperature":       0.0,
	}
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/responses", payload)
	var deltaSeen, completedSeen bool
	var captured []SSEEvent
	err := sc.Client.StreamPostJSON(ctx, "/v1/responses", payload, nil, false, func(ev SSEEvent) {
		captured = append(captured, ev)
		evName := strings.TrimSpace(ev.Event)
		if strings.HasSuffix(evName, "output_text.delta") || strings.HasSuffix(evName, "delta") {
			deltaSeen = true
		}
		if evName == "response.completed" || strings.HasSuffix(evName, "completed") {
			completedSeen = true
		}
	})
	respTrace := sseTraceFromEvents(captured, 200)
	events := len(captured)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, respTrace)}
	}
	if events == 0 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "no SSE events received", ""), reqTrace, respTrace)}
	}
	if !deltaSeen {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "no streaming delta received", fmt.Sprintf("events=%d", events)), reqTrace, respTrace)}
	}
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("events=%d completed=%v", events, completedSeen)), reqTrace, respTrace)}
}

// SuiteClaude exercises /v1/messages tool_use with the complex input schema.
func SuiteClaude(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/messages (tool_use)"
	payload := map[string]interface{}{
		"model":      sc.ClaudeModel,
		"max_tokens": 1024,
		"system":     "You are a travel-planning assistant. When asked for an itinerary, always call the record_travel_itinerary tool with the structured details. Do not respond with prose.",
		"messages": []map[string]interface{}{
			{"role": "user", "content": TravelItineraryPrompt()},
		},
		"tools": []map[string]interface{}{
			{
				"name":         "record_travel_itinerary",
				"description":  "Persist the structured travel itinerary for the user.",
				"input_schema": TravelItinerarySchema(),
			},
		},
		"tool_choice": map[string]interface{}{
			"type": "tool",
			"name": "record_travel_itinerary",
		},
		"temperature": 0.0,
	}
	extra := make(http.Header)
	extra.Set("anthropic-version", "2023-06-01")
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/messages", payload)
	reqTrace.Headers = map[string]string{"anthropic-version": "2023-06-01"}
	resp, err := sc.Client.PostJSON(ctx, "/v1/messages", payload, extra, false)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, nil)}
	}
	respTrace := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	content := gjson.GetBytes(resp.Body, "content").Array()
	if len(content) == 0 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "missing content array", truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	var toolUse *gjson.Result
	for i := range content {
		if content[i].Get("type").String() == "tool_use" {
			block := content[i]
			toolUse = &block
			break
		}
	}
	if toolUse == nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "no tool_use block in response", truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	if n := toolUse.Get("name").String(); n != "record_travel_itinerary" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("unexpected tool name %q", n), ""), reqTrace, respTrace)}
	}
	inputRaw := toolUse.Get("input").Raw
	if strings.TrimSpace(inputRaw) == "" || inputRaw == "null" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "tool_use.input is missing", ""), reqTrace, respTrace)}
	}
	parsed, err := parseLoose(inputRaw)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("tool_use.input not JSON: %v", err), ""), reqTrace, respTrace)}
	}
	if schemaErr := ValidateTravelItinerary(parsed); schemaErr != "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), schemaErr, ""), reqTrace, respTrace)}
	}
	tripID := gjson.Get(inputRaw, "trip_id").String()
	legs := gjson.Get(inputRaw, "legs").Array()
	return []CheckResult{attach(Pass(name, sw.Elapsed(), fmt.Sprintf("trip_id=%q legs=%d", tripID, len(legs))), reqTrace, respTrace)}
}

// SuiteImages exercises POST /v1/images/generations with an image-capable
// model (defaults to gpt-image-2). The response contains base64-encoded
// image data, which the dashboard auto-renders into the detail panel.
func SuiteImages(ctx context.Context, sc *SuiteContext) []CheckResult {
	name := "POST /v1/images/generations"
	model := strings.TrimSpace(sc.ImagesModel)
	if model == "" {
		model = "gpt-image-2"
	}
	payload := map[string]interface{}{
		"model":           model,
		"prompt":          "A minimalist vector map of Japan highlighting Tokyo, Osaka and Kyoto with small dots and labels. Soft pastel palette, clean white background.",
		"n":               1,
		"size":            "1024x1024",
		"response_format": "b64_json",
	}
	sw := StartStopwatch()
	reqTrace := jsonReq("POST", "/v1/images/generations", payload)
	resp, err := sc.Client.PostJSON(ctx, "/v1/images/generations", payload, nil, false)
	if err != nil {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), err.Error(), ""), reqTrace, nil)}
	}
	respTrace := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	first := gjson.GetBytes(resp.Body, "data.0")
	if !first.Exists() {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "response.data[0] missing", truncate(resp.Text(), 200)), reqTrace, respTrace)}
	}
	b64 := first.Get("b64_json").String()
	url := first.Get("url").String()
	if b64 == "" && url == "" {
		return []CheckResult{attach(Fail(name, sw.Elapsed(), "response.data[0] has neither b64_json nor url", ""), reqTrace, respTrace)}
	}
	detail := fmt.Sprintf("model=%s size=1024x1024", model)
	if b64 != "" {
		detail += fmt.Sprintf(" b64_bytes=%d", len(b64))
	}
	if url != "" {
		detail += " url=" + truncate(url, 64)
	}
	return []CheckResult{attach(Pass(name, sw.Elapsed(), detail), reqTrace, respTrace)}
}

// SuiteGemini exercises /v1beta/models and :generateContent with responseSchema.
func SuiteGemini(ctx context.Context, sc *SuiteContext) []CheckResult {
	results := make([]CheckResult, 0, 2)

	listName := "GET /v1beta/models"
	listSW := StartStopwatch()
	listReq := &Trace{Method: "GET", URL: "/v1beta/models"}
	listResp, err := sc.Client.Get(ctx, "/v1beta/models", nil, true)
	if err != nil {
		results = append(results, attach(Fail(listName, listSW.Elapsed(), err.Error(), ""), listReq, nil))
	} else {
		listTrace := traceFromResponse(listResp)
		if listResp.StatusCode != 200 {
			results = append(results, attach(Fail(listName, listSW.Elapsed(), fmt.Sprintf("status %d", listResp.StatusCode), truncate(listResp.Text(), 200)), listReq, listTrace))
		} else {
			models := gjson.GetBytes(listResp.Body, "models").Array()
			if len(models) == 0 {
				results = append(results, attach(Fail(listName, listSW.Elapsed(), "response.models empty or missing", truncate(listResp.Text(), 200)), listReq, listTrace))
			} else {
				results = append(results, attach(Pass(listName, listSW.Elapsed(), fmt.Sprintf("%d gemini models", len(models))), listReq, listTrace))
			}
		}
	}

	genName := "POST /v1beta/models/{m}:generateContent (responseSchema)"
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role":  "user",
				"parts": []map[string]interface{}{{"text": TravelItineraryPrompt()}},
			},
		},
		"systemInstruction": map[string]interface{}{
			"role": "system",
			"parts": []map[string]interface{}{
				{"text": "You are a travel-planning assistant. Always reply with the structured itinerary requested by the user. Never include explanations or text outside the structured payload."},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json",
			"responseSchema":   TravelItinerarySchemaForGemini(),
			"temperature":      0.0,
			"maxOutputTokens":  1024,
		},
	}
	genSW := StartStopwatch()
	path := fmt.Sprintf("/v1beta/models/%s:generateContent", sc.GeminiModel)
	genReq := jsonReq("POST", path, payload)
	resp, err := sc.Client.PostJSON(ctx, path, payload, nil, true)
	if err != nil {
		results = append(results, attach(Fail(genName, genSW.Elapsed(), err.Error(), ""), genReq, nil))
		return results
	}
	genResp := traceFromResponse(resp)
	if resp.StatusCode != 200 {
		results = append(results, attach(Fail(genName, genSW.Elapsed(), fmt.Sprintf("status %d", resp.StatusCode), truncate(resp.Text(), 200)), genReq, genResp))
		return results
	}
	text := extractGeminiText(resp.Body)
	if strings.TrimSpace(text) == "" {
		results = append(results, attach(Fail(genName, genSW.Elapsed(), "empty candidate text", truncate(resp.Text(), 200)), genReq, genResp))
		return results
	}
	parsed, err := parseLoose(text)
	if err != nil {
		results = append(results, attach(Fail(genName, genSW.Elapsed(), fmt.Sprintf("non-JSON content: %v", err), truncate(text, 200)), genReq, genResp))
		return results
	}
	if schemaErr := ValidateTravelItinerary(parsed); schemaErr != "" {
		results = append(results, attach(Fail(genName, genSW.Elapsed(), schemaErr, ""), genReq, genResp))
		return results
	}
	tripID := gjson.Get(text, "trip_id").String()
	legs := gjson.Get(text, "legs").Array()
	results = append(results, attach(Pass(genName, genSW.Elapsed(), fmt.Sprintf("trip_id=%q legs=%d", tripID, len(legs))), genReq, genResp))
	return results
}

// -------------------------- helpers --------------------------

func structuredMessages() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"role":    "system",
			"content": "You are a travel-planning assistant. Always reply with the structured itinerary requested by the user. Never include explanations, markdown, or text outside the structured payload.",
		},
		{
			"role":    "user",
			"content": TravelItineraryPrompt(),
		},
	}
}

// parseLoose accepts a JSON string (possibly wrapped in markdown code fences)
// and decodes it into a generic structure.
func parseLoose(s string) (interface{}, error) {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```") {
		// Strip the first and last fence.
		first := strings.Index(trimmed, "\n")
		if first >= 0 {
			trimmed = trimmed[first+1:]
		}
		if idx := strings.LastIndex(trimmed, "```"); idx >= 0 {
			trimmed = trimmed[:idx]
		}
		trimmed = strings.TrimSpace(trimmed)
	}
	var out interface{}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// extractResponsesText walks a non-streaming /v1/responses payload for textual output.
func extractResponsesText(body []byte) string {
	direct := gjson.GetBytes(body, "output_text").String()
	if strings.TrimSpace(direct) != "" {
		return direct
	}
	out := gjson.GetBytes(body, "output").Array()
	var parts []string
	for _, item := range out {
		content := item.Get("content")
		if !content.Exists() {
			continue
		}
		if content.IsArray() {
			for _, piece := range content.Array() {
				if t := piece.Get("text").String(); t != "" {
					parts = append(parts, t)
					continue
				}
				if t := piece.Get("output_text").String(); t != "" {
					parts = append(parts, t)
					continue
				}
				if t := piece.Get("text.value").String(); t != "" {
					parts = append(parts, t)
				}
			}
		} else if s := content.String(); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "")
}

// extractGeminiText pulls candidate text from a :generateContent response.
func extractGeminiText(body []byte) string {
	candidates := gjson.GetBytes(body, "candidates").Array()
	if len(candidates) == 0 {
		return ""
	}
	parts := candidates[0].Get("content.parts").Array()
	var out []string
	for _, part := range parts {
		if t := part.Get("text").String(); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "")
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// -------------------------- trace helpers --------------------------

// attach wires request/response traces into a CheckResult.
func attach(r CheckResult, req, resp *Trace) CheckResult {
	return r.WithRequest(req).WithResponse(resp)
}

// jsonReq produces a Trace for an outgoing JSON POST request. The body is
// marshalled with indentation for readability.
func jsonReq(method, url string, body interface{}) *Trace {
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return &Trace{Method: method, URL: url, Kind: "json", Body: fmt.Sprintf("<marshal error: %v>", err)}
	}
	return &Trace{Method: method, URL: url, Kind: "json", Body: string(raw)}
}

// traceFromResponse captures the non-streaming response for a Trace.
func traceFromResponse(resp *Response) *Trace {
	if resp == nil {
		return nil
	}
	kind := "json"
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(ct, "event-stream"):
		kind = "sse"
	case strings.Contains(ct, "text/") && !strings.Contains(ct, "json"):
		kind = "text"
	case ct == "":
		kind = "text"
	}
	return &Trace{
		Status: resp.StatusCode,
		Body:   string(resp.Body),
		Kind:   kind,
	}
}

// sseTraceFromEvents reconstructs an SSE-style body string from a slice of
// parsed events. Keeps the latest maxEvents entries to avoid huge payloads.
func sseTraceFromEvents(events []SSEEvent, maxEvents int) *Trace {
	if maxEvents <= 0 || len(events) <= maxEvents {
		return &Trace{Kind: "sse", Body: formatEvents(events)}
	}
	trimmed := events[len(events)-maxEvents:]
	preamble := fmt.Sprintf("(omitted %d earlier events)\n\n", len(events)-maxEvents)
	return &Trace{Kind: "sse", Body: preamble + formatEvents(trimmed)}
}

func formatEvents(events []SSEEvent) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Event != "" {
			b.WriteString("event: ")
			b.WriteString(ev.Event)
			b.WriteByte('\n')
		}
		if ev.Data != "" {
			for _, line := range strings.Split(ev.Data, "\n") {
				b.WriteString("data: ")
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
