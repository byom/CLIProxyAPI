# AI Agent Integration Guide

English | [中文](ai-integration_CN.md)

> Audience: an **AI coding agent** (Claude Code, Cursor, Codex CLI, Cline, etc.)
> that needs to wire an arbitrary project to a running **CLIProxyAPI** server **with
> zero human intervention**.
>
> This document is intentionally prescriptive and machine-friendly. An agent that
> follows the checklist in §10 can finish the integration in one pass without
> asking the user.

---

## 1. What CLIProxyAPI exposes

CLIProxyAPI is a local HTTP proxy that speaks **four upstream-compatible
protocols on the same port** and fans out to OAuth-logged-in accounts and
API keys:

| Protocol family | Path prefix                      | Drop-in replacement for |
| --------------- | -------------------------------- | ----------------------- |
| OpenAI          | `/v1/chat/completions`, `/v1/completions`, `/v1/responses`, `/v1/images/*`, `/v1/models` | `https://api.openai.com` |
| Anthropic       | `/v1/messages`, `/v1/messages/count_tokens`, `/v1/models` | `https://api.anthropic.com` |
| Google Gemini   | `/v1beta/models`, `/v1beta/models/{model}:generateContent`, `:streamGenerateContent` | `https://generativelanguage.googleapis.com` |
| OpenAI Codex    | `/backend-api/codex/responses` | ChatGPT `chatgpt_base_url` |

One single API key (from `api-keys:` in `config.yaml`) authenticates all four
surfaces. Therefore the integration for any SDK boils down to **overriding the
base URL and the API key**.

---

## 2. Connection parameters (what the agent must discover)

| Parameter   | Default                 | Resolution order                                                                 |
| ----------- | ----------------------- | -------------------------------------------------------------------------------- |
| Base URL    | `http://localhost:8317` | `$CLIPROXY_BASE_URL` → project-local `.env`/config → default                    |
| API key     | *(required)*            | `$CLIPROXY_API_KEY` → project-local `.env` → ask user **only if all fail**      |
| TLS         | off                     | If base URL begins with `https://`, assume self-signed may be present           |

### 2.1 Probe script (language-agnostic)

```bash
BASE="${CLIPROXY_BASE_URL:-http://localhost:8317}"
curl -fsS "$BASE/healthz"                  # → {"status":"ok"}
curl -fsS -H "Authorization: Bearer $CLIPROXY_API_KEY" "$BASE/v1/models" | jq '.data[].id' | head
```

If `/healthz` returns `200 {"status":"ok"}` the proxy is reachable.
If `/v1/models` returns `401` the API key is wrong.
If `/v1/models` returns the list, integration can proceed immediately.

---

## 3. Authentication — pick **any one** header

The proxy accepts all of these simultaneously; use whichever matches the SDK
you are integrating:

| Style      | Header / query                        | Use for                              |
| ---------- | ------------------------------------- | ------------------------------------ |
| OpenAI     | `Authorization: Bearer <KEY>`         | OpenAI SDKs, LiteLLM, Continue, etc. |
| Anthropic  | `x-api-key: <KEY>`                    | `anthropic` SDK, Claude Code         |
| Gemini     | `x-goog-api-key: <KEY>` or `?key=<KEY>` | `google-genai`, Gemini CLI           |
| Generic    | `?auth_token=<KEY>`                   | Websocket / EventSource fallback     |

> Do **not** strip existing `Authorization` headers when forwarding from
> frameworks — the proxy resolves them transparently.

---

## 4. Endpoint reference (complete)

### 4.1 OpenAI-compatible (`/v1`)

| Method | Path                         | Body schema                                 | Streaming          |
| ------ | ---------------------------- | ------------------------------------------- | ------------------ |
| GET    | `/v1/models`                 | —                                           | —                  |
| POST   | `/v1/chat/completions`       | OpenAI Chat Completions                     | `{"stream": true}` → SSE |
| POST   | `/v1/completions`            | OpenAI legacy text completion               | `{"stream": true}` → SSE |
| POST   | `/v1/responses`              | OpenAI Responses API                        | `{"stream": true}` → SSE |
| POST   | `/v1/responses/compact`      | Responses compact summarize                 | Non-stream only    |
| POST   | `/v1/images/generations`     | OpenAI images                               | Non-stream only    |
| POST   | `/v1/images/edits`           | OpenAI images edit (multipart)              | Non-stream only    |

### 4.2 Anthropic-compatible (`/v1`)

| Method | Path                                | Body schema                | Streaming           |
| ------ | ----------------------------------- | -------------------------- | ------------------- |
| POST   | `/v1/messages`                      | Anthropic Messages         | `{"stream": true}` → SSE |
| POST   | `/v1/messages/count_tokens`         | Anthropic Count-Tokens     | Non-stream only     |

> `GET /v1/models` routes to the Anthropic-style list when `User-Agent` starts
> with `claude-cli/`; otherwise returns the OpenAI-style list. Both lists
> contain the same underlying model IDs.

### 4.3 Google Gemini-compatible (`/v1beta`)

| Method | Path                                             | Body schema                      |
| ------ | ------------------------------------------------ | -------------------------------- |
| GET    | `/v1beta/models`                                 | —                                |
| POST   | `/v1beta/models/{model}:generateContent`         | Gemini GenerateContent           |
| POST   | `/v1beta/models/{model}:streamGenerateContent?alt=sse` | same (streamed via SSE)    |
| POST   | `/v1beta/models/{model}:countTokens`             | Gemini CountTokens               |

### 4.4 Codex CLI aliases (`/backend-api/codex`)

Use these when the upstream client is the official Codex CLI and you want
`chatgpt_base_url` compatibility:

- `POST /backend-api/codex/responses`
- `GET  /backend-api/codex/responses` (WebSocket upgrade)
- `POST /backend-api/codex/responses/compact`

### 4.5 Health & root

- `GET /healthz` — unauthenticated liveness probe, returns `{"status":"ok"}`.
- `GET /` — JSON describing the most common endpoints.

---

## 5. Model discovery (always do this before first call)

```bash
curl -s -H "Authorization: Bearer $CLIPROXY_API_KEY" \
  "$CLIPROXY_BASE_URL/v1/models" | jq '.data[].id'
```

Typical IDs returned look like:
- `gpt-5`, `gpt-5-codex`, `gpt-4o`, …
- `claude-sonnet-4-5-20250929`, `claude-opus-4-5-20251101`, `claude-haiku-4-5-20251001`, …
- `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-3-pro-preview`, …
- Any user-defined aliases from `config.yaml` (e.g. `codex-latest`, `claude-sonnet-latest`).

**Always pick a model from this list**; do not hardcode model names that the
deployment might not expose.

---

## 6. Ready-to-paste SDK snippets

### 6.1 Node.js — `openai`

```js
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: process.env.CLIPROXY_BASE_URL ?? "http://localhost:8317/v1",
  apiKey: process.env.CLIPROXY_API_KEY,
});

const r = await client.chat.completions.create({
  model: "gpt-5",
  messages: [{ role: "user", content: "hi" }],
  stream: false,
});
console.log(r.choices[0].message.content);
```

### 6.2 Python — `openai`

```python
import os
from openai import OpenAI

client = OpenAI(
    base_url=os.getenv("CLIPROXY_BASE_URL", "http://localhost:8317") + "/v1",
    api_key=os.environ["CLIPROXY_API_KEY"],
)
r = client.chat.completions.create(
    model="gpt-5",
    messages=[{"role": "user", "content": "hi"}],
)
print(r.choices[0].message.content)
```

### 6.3 Python — `anthropic`

```python
import os
from anthropic import Anthropic

client = Anthropic(
    base_url=os.getenv("CLIPROXY_BASE_URL", "http://localhost:8317"),
    api_key=os.environ["CLIPROXY_API_KEY"],
)
msg = client.messages.create(
    model="claude-sonnet-4-5-20250929",
    max_tokens=1024,
    messages=[{"role": "user", "content": "hi"}],
)
print(msg.content[0].text)
```

### 6.4 Python — `google-genai`

```python
import os
from google import genai

client = genai.Client(
    api_key=os.environ["CLIPROXY_API_KEY"],
    http_options={"base_url": os.getenv("CLIPROXY_BASE_URL", "http://localhost:8317")},
)
resp = client.models.generate_content(
    model="gemini-2.5-flash",
    contents="hi",
)
print(resp.text)
```

### 6.5 curl / fetch (no SDK)

```bash
curl -N -X POST "$CLIPROXY_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $CLIPROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5",
    "stream": true,
    "messages": [{"role":"user","content":"hi"}]
  }'
```

---

## 7. Streaming semantics

| Protocol    | Transport                         | Terminator                         |
| ----------- | --------------------------------- | ---------------------------------- |
| OpenAI      | `text/event-stream`, `data: {...}` lines | `data: [DONE]`                    |
| Anthropic   | `text/event-stream`, `event: ...` + `data: {...}` | `event: message_stop`    |
| Gemini SSE  | `text/event-stream`, `data: {...}` | connection close                   |

Do **not** set a read timeout on streams after the first byte; the proxy has
already handled upstream timeouts.

---

## 8. Automated configuration for popular AI clients

When an AI agent wires an existing tool to the proxy, write the following
environment / config. All values reuse `$CLIPROXY_BASE_URL` and
`$CLIPROXY_API_KEY`.

### 8.1 Claude Code (Anthropic CLI)

```bash
export ANTHROPIC_BASE_URL="$CLIPROXY_BASE_URL"
export ANTHROPIC_AUTH_TOKEN="$CLIPROXY_API_KEY"
export ANTHROPIC_MODEL="claude-sonnet-4-5-20250929"        # any ID from /v1/models
# optional: export ANTHROPIC_SMALL_FAST_MODEL="claude-haiku-4-5-20251001"
```

### 8.2 Codex CLI / ChatGPT CLI

```bash
export OPENAI_API_BASE="$CLIPROXY_BASE_URL/v1"
export OPENAI_API_KEY="$CLIPROXY_API_KEY"
# or, for Codex CLI's native websocket path:
export CHATGPT_BASE_URL="$CLIPROXY_BASE_URL/backend-api/codex"
```

### 8.3 Gemini CLI / `google-genai`

```bash
export GOOGLE_GEMINI_BASE_URL="$CLIPROXY_BASE_URL"
export GEMINI_API_KEY="$CLIPROXY_API_KEY"
```

### 8.4 Cursor / Continue.dev / Cline (OpenAI-compatible provider)

Register a custom provider whose base URL is `$CLIPROXY_BASE_URL/v1` and key is
`$CLIPROXY_API_KEY`. Use a model ID returned by `/v1/models`.

### 8.5 LangChain / LlamaIndex

Use the OpenAI, Anthropic, or Google chat model classes with `base_url` /
`api_key` overridden exactly as in §6.

---

## 9. Error handling & retries

| HTTP status | Meaning                            | Agent action                                          |
| ----------- | ---------------------------------- | ----------------------------------------------------- |
| 200         | success                            | —                                                     |
| 400         | malformed request                  | fix request body; do **not** retry blindly            |
| 401         | missing / invalid API key          | re-read `$CLIPROXY_API_KEY`; surface to user if absent |
| 403         | auth blocked (e.g. remote mgmt off)| stop; only local clients are permitted for that route |
| 404         | unknown path / model               | re-run `/v1/models`; use a valid ID                   |
| 408/429     | upstream rate-limited              | retry with exponential backoff (server already retries internally up to `request-retry`) |
| 5xx         | upstream error                     | retry once, then fall back to another model           |

The proxy itself retries idempotent upstream calls (`request-retry`, default 3)
and rotates credentials automatically, so client-side retry loops should stay
short (≤ 2 attempts).

---

## 10. Integration checklist — zero-human-intervention flow

An AI agent integrating a project **must** execute the following in order:

1. **Locate the proxy**
   - Read `CLIPROXY_BASE_URL`, fallback `http://localhost:8317`.
   - `GET /healthz` → expect `200 {"status":"ok"}`. If it fails, the proxy is
     not running; stop and report to the user.
2. **Acquire the API key**
   - Read `CLIPROXY_API_KEY`; fallback to project `.env`.
   - If neither is set, look in `config.yaml` → `api-keys:` (first entry);
     only prompt the user if **all three** sources are empty.
3. **Verify auth**
   - `GET /v1/models` with `Authorization: Bearer <key>`.
   - On `401`, stop and prompt the user.
4. **Select a model**
   - Filter `/v1/models` `data[].id` by the task:
     - tool use & long context → prefer `claude-sonnet-*` or `gpt-5`
     - cheap / fast → `gemini-2.5-flash`, `claude-haiku-*`, `gpt-5-mini`
     - image generation → any model exposing `/v1/images/generations`
5. **Inject configuration** into the target project:
   - Prefer environment variables (§8).
   - When the project already has a config file (`.continuerc`, `mcp.json`,
     `cursor settings.json`, …), patch it to point `baseURL` / `apiKey` /
     `model` at the proxy. **Do not remove** the user's other providers;
     append a new one named `cliproxy`.
6. **Smoke test**
   - Fire one non-streaming chat request (§6). On HTTP 200, emit the success
     message and finish.
7. **Record what was changed** in a short summary so the user can audit.

If step 1 or 2 fails, the agent must ask the user for the missing value; in
all other cases the integration is fully automatic.

---

## 11. Advanced behavior worth knowing

- **Model aliases & prefixes.** Operators may define aliases like `test/gpt-5`
  to pin a request to a specific credential. If the model list contains IDs
  with `/` in them, treat the slash as an opaque character — pass the ID
  through verbatim.
- **Session affinity.** If `routing.session-affinity: true`, the proxy binds
  a conversation to a single upstream account via `X-Session-ID`,
  `Idempotency-Key`, `metadata.user_id`, or hashed first messages. For most
  integrations no action is required.
- **Passthrough headers.** Custom headers sent by the client (e.g. Anthropic
  beta flags) are forwarded unless stripped by a per-provider filter.
- **Thinking / reasoning.** All reasoning-config surfaces (OpenAI
  `reasoning.effort`, Anthropic `thinking`, Gemini `thinkingConfig`) are
  normalized internally; send them in whatever shape the SDK supports.
- **Management API.** `/v0/management/*` is disabled unless
  `remote-management.secret-key` is configured. AI integrations should not
  touch it.

---

## 12. One-liner verification

Drop this at the end of your integration for a quick self-check:

```bash
curl -fsS -H "Authorization: Bearer $CLIPROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5","messages":[{"role":"user","content":"ping"}]}' \
  "$CLIPROXY_BASE_URL/v1/chat/completions" | jq -r '.choices[0].message.content'
```

If this prints a non-empty reply, the project is fully integrated.
