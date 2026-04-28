package modeltest

// Registry exposes the set of named suites available to the runner and the
// management API. Callers should treat it as read-only.
var Registry = map[string]Suite{
	"models": {
		Name:        "models",
		Description: "GET /v1/models",
		Run:         SuiteModels,
	},
	"chat": {
		Name:        "chat",
		Description: "POST /v1/chat/completions (plain)",
		Run:         SuiteChatPlain,
	},
	"chat_json": {
		Name:        "chat_json",
		Description: "POST /v1/chat/completions with response_format=json_schema",
		Run:         SuiteChatJSON,
	},
	"chat_tools": {
		Name:        "chat_tools",
		Description: "POST /v1/chat/completions with forced tool call",
		Run:         SuiteChatTools,
	},
	"chat_stream": {
		Name:        "chat_stream",
		Description: "POST /v1/chat/completions streaming",
		Run:         SuiteChatStream,
	},
	"responses": {
		Name:        "responses",
		Description: "POST /v1/responses with structured text.format",
		Run:         SuiteResponsesStructured,
	},
	"responses_stream": {
		Name:        "responses_stream",
		Description: "POST /v1/responses streaming",
		Run:         SuiteResponsesStream,
	},
	"claude": {
		Name:        "claude",
		Description: "POST /v1/messages with tool_use input_schema",
		Run:         SuiteClaude,
	},
	"gemini": {
		Name:        "gemini",
		Description: "GET /v1beta/models + generateContent with responseSchema",
		Run:         SuiteGemini,
	},
	"images": {
		Name:        "images",
		Description: "POST /v1/images/generations (image generation model)",
		Run:         SuiteImages,
	},
}

// SuiteOrder returns the canonical ordered list of suite names. The order
// matches the original api_smoke ordering (plus `images` at the end) so UI
// renderings remain stable.
func SuiteOrder() []string {
	return []string{
		"models",
		"chat",
		"chat_json",
		"chat_tools",
		"chat_stream",
		"responses",
		"responses_stream",
		"claude",
		"gemini",
		"images",
	}
}

// SuiteNames returns an ordered slice of registered suite names.
func SuiteNames() []string {
	ordered := SuiteOrder()
	out := make([]string, 0, len(ordered))
	for _, name := range ordered {
		if _, ok := Registry[name]; ok {
			out = append(out, name)
		}
	}
	return out
}
