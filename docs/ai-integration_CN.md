# AI Agent 接入指南

[English](ai-integration.md) | 中文

> 读者：**AI 编码代理**（Claude Code、Cursor、Codex CLI、Cline 等），
> 需要把一个任意的工程**零人工参与**地接入到一台正在运行的 **CLIProxyAPI** 上。
>
> 本文档以"命令式 + 机器友好"的风格编写。AI 只要按照 §10 的清单走一遍，
> 就能一次完成接入，不需要向用户追问任何信息（除非代理不可达或密钥缺失）。

---

## 1. CLIProxyAPI 提供什么

CLIProxyAPI 是一个本地 HTTP 反向代理，在**同一个端口**上同时暴露四套上游兼容协议，
并将请求分发到 OAuth 登录的账号或 API Key：

| 协议族       | 路径前缀                                                                                          | 等价替换                                       |
| ------------ | ------------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| OpenAI       | `/v1/chat/completions`、`/v1/completions`、`/v1/responses`、`/v1/images/*`、`/v1/models`          | `https://api.openai.com`                       |
| Anthropic    | `/v1/messages`、`/v1/messages/count_tokens`、`/v1/models`                                         | `https://api.anthropic.com`                    |
| Google Gemini | `/v1beta/models`、`/v1beta/models/{model}:generateContent`、`:streamGenerateContent`             | `https://generativelanguage.googleapis.com`    |
| OpenAI Codex | `/backend-api/codex/responses`                                                                    | ChatGPT 的 `chatgpt_base_url`                  |

一把 API Key（来自 `config.yaml` 的 `api-keys:`）即可鉴权所有四个接口面。
因此，任何 SDK 的接入归结为**覆盖 base URL 与 API Key 两项**即可。

---

## 2. 连接参数（AI 需要自动发现的量）

| 参数       | 默认值                   | 解析顺序                                                                         |
| ---------- | ------------------------ | -------------------------------------------------------------------------------- |
| Base URL   | `http://localhost:8317`  | `$CLIPROXY_BASE_URL` → 工程内 `.env`/配置 → 默认值                             |
| API Key    | *（必需）*               | `$CLIPROXY_API_KEY` → 工程内 `.env` → 全部缺失时**再**询问用户                  |
| TLS        | 关闭                     | 如果 Base URL 以 `https://` 开头，需允许自签证书                                 |

### 2.1 通用探测脚本

```bash
BASE="${CLIPROXY_BASE_URL:-http://localhost:8317}"
curl -fsS "$BASE/healthz"                  # → {"status":"ok"}
curl -fsS -H "Authorization: Bearer $CLIPROXY_API_KEY" "$BASE/v1/models" | jq '.data[].id' | head
```

- `/healthz` 返回 `200 {"status":"ok"}` → 代理可达。
- `/v1/models` 返回 `401` → API Key 不对。
- `/v1/models` 正常返回 → 接入条件具备，可以立即开始。

---

## 3. 鉴权方式（三选一，任意可用）

代理同时接受以下所有风格的凭证，选择与目标 SDK 最匹配的那一种即可：

| 风格      | Header / Query                        | 常见用途                             |
| --------- | ------------------------------------- | ------------------------------------ |
| OpenAI    | `Authorization: Bearer <KEY>`         | OpenAI SDK、LiteLLM、Continue 等     |
| Anthropic | `x-api-key: <KEY>`                    | `anthropic` SDK、Claude Code         |
| Gemini    | `x-goog-api-key: <KEY>` 或 `?key=<KEY>` | `google-genai`、Gemini CLI           |
| 通用      | `?auth_token=<KEY>`                   | WebSocket / EventSource 场景兜底     |

> 转发框架里**不要**把已有的 `Authorization` 头剥离，代理会自动识别。

---

## 4. 全部可用端点

### 4.1 OpenAI 兼容（`/v1`）

| 方法 | 路径                         | Body 协议                                     | 流式                        |
| ---- | ---------------------------- | --------------------------------------------- | --------------------------- |
| GET  | `/v1/models`                 | —                                             | —                           |
| POST | `/v1/chat/completions`       | OpenAI Chat Completions                       | `{"stream": true}` → SSE    |
| POST | `/v1/completions`            | OpenAI 旧版文本补全                           | `{"stream": true}` → SSE    |
| POST | `/v1/responses`              | OpenAI Responses API                          | `{"stream": true}` → SSE    |
| POST | `/v1/responses/compact`      | Responses compact summarize                   | 仅非流式                    |
| POST | `/v1/images/generations`     | OpenAI Images                                 | 仅非流式                    |
| POST | `/v1/images/edits`           | OpenAI Images edit（multipart）               | 仅非流式                    |

### 4.2 Anthropic 兼容（`/v1`）

| 方法 | 路径                          | Body 协议              | 流式                        |
| ---- | ----------------------------- | ---------------------- | --------------------------- |
| POST | `/v1/messages`                | Anthropic Messages     | `{"stream": true}` → SSE    |
| POST | `/v1/messages/count_tokens`   | Anthropic CountTokens  | 仅非流式                    |

> `GET /v1/models`：当 `User-Agent` 以 `claude-cli/` 开头时返回 Anthropic 风格的列表，
> 否则返回 OpenAI 风格列表；两者所含的模型 ID 相同。

### 4.3 Google Gemini 兼容（`/v1beta`）

| 方法 | 路径                                                   | Body 协议                     |
| ---- | ------------------------------------------------------ | ----------------------------- |
| GET  | `/v1beta/models`                                       | —                             |
| POST | `/v1beta/models/{model}:generateContent`               | Gemini GenerateContent        |
| POST | `/v1beta/models/{model}:streamGenerateContent?alt=sse` | 同上，流式 SSE 输出           |
| POST | `/v1beta/models/{model}:countTokens`                   | Gemini CountTokens            |

### 4.4 Codex CLI 专用别名（`/backend-api/codex`）

当上游客户端是官方 Codex CLI 且要求 `chatgpt_base_url` 兼容时使用：

- `POST /backend-api/codex/responses`
- `GET  /backend-api/codex/responses`（WebSocket 升级）
- `POST /backend-api/codex/responses/compact`

### 4.5 健康检查 & 根路由

- `GET /healthz` —— 无需鉴权，返回 `{"status":"ok"}`。
- `GET /` —— 返回一个 JSON，内含最常用端点。

---

## 5. 首次调用前务必先做"模型发现"

```bash
curl -s -H "Authorization: Bearer $CLIPROXY_API_KEY" \
  "$CLIPROXY_BASE_URL/v1/models" | jq '.data[].id'
```

常见的模型 ID：
- `gpt-5`、`gpt-5-codex`、`gpt-4o`……
- `claude-sonnet-4-5-20250929`、`claude-opus-4-5-20251101`、`claude-haiku-4-5-20251001`……
- `gemini-2.5-pro`、`gemini-2.5-flash`、`gemini-3-pro-preview`……
- 来自 `config.yaml` 的自定义别名（例如 `codex-latest`、`claude-sonnet-latest`）。

**永远从实际返回的列表中选取模型**，不要硬编码某个部署可能并未开放的模型名。

---

## 6. 可直接拷贝的 SDK 接入片段

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

### 6.5 纯 curl / fetch（无 SDK）

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

## 7. 流式协议细节

| 协议        | 传输                                       | 结束标记                         |
| ----------- | ------------------------------------------ | -------------------------------- |
| OpenAI      | `text/event-stream`，`data: {...}`         | `data: [DONE]`                   |
| Anthropic   | `text/event-stream`，`event: ...` + `data: {...}` | `event: message_stop`     |
| Gemini SSE  | `text/event-stream`，`data: {...}`         | 连接关闭                         |

**在收到首字节后不要设置读超时**——上游超时已由代理统一处理。

---

## 8. 常见 AI 客户端的一键配置

AI 接入已有工具时，优先以环境变量形式写入，所有值共享 `$CLIPROXY_BASE_URL` 与
`$CLIPROXY_API_KEY`。

### 8.1 Claude Code（Anthropic CLI）

```bash
export ANTHROPIC_BASE_URL="$CLIPROXY_BASE_URL"
export ANTHROPIC_AUTH_TOKEN="$CLIPROXY_API_KEY"
export ANTHROPIC_MODEL="claude-sonnet-4-5-20250929"        # 必须来自 /v1/models
# 可选： export ANTHROPIC_SMALL_FAST_MODEL="claude-haiku-4-5-20251001"
```

### 8.2 Codex CLI / ChatGPT CLI

```bash
export OPENAI_API_BASE="$CLIPROXY_BASE_URL/v1"
export OPENAI_API_KEY="$CLIPROXY_API_KEY"
# 使用 Codex CLI 原生 WebSocket 路径时：
export CHATGPT_BASE_URL="$CLIPROXY_BASE_URL/backend-api/codex"
```

### 8.3 Gemini CLI / `google-genai`

```bash
export GOOGLE_GEMINI_BASE_URL="$CLIPROXY_BASE_URL"
export GEMINI_API_KEY="$CLIPROXY_API_KEY"
```

### 8.4 Cursor / Continue.dev / Cline（任何 OpenAI 兼容提供方）

注册自定义 Provider：base URL = `$CLIPROXY_BASE_URL/v1`，API Key =
`$CLIPROXY_API_KEY`，模型取 `/v1/models` 中任一 ID。

### 8.5 LangChain / LlamaIndex

使用 OpenAI / Anthropic / Google 对应的聊天模型类，按 §6 的写法覆盖
`base_url` 与 `api_key` 即可。

---

## 9. 错误码与重试策略

| HTTP 状态 | 含义                          | AI 处理方式                                                     |
| --------- | ----------------------------- | ----------------------------------------------------------------- |
| 200       | 成功                          | —                                                                 |
| 400       | 请求体错误                    | 修正 body；**不要**盲目重试                                      |
| 401       | 缺失 / 错误的 API Key         | 重新读取 `$CLIPROXY_API_KEY`；仍无则提示用户                    |
| 403       | 权限不足（如远程管理未开启）  | 停止操作；该路由仅允许本地客户端                                 |
| 404       | 路径 / 模型不存在             | 重新调用 `/v1/models`，使用真实 ID                               |
| 408 / 429 | 上游限流                      | 指数退避重试（代理内部已按 `request-retry` 重试过）              |
| 5xx       | 上游异常                      | 重试 1 次后回退到其他模型                                        |

代理内部已做幂等请求重试（默认 3 次）并自动轮换凭据，因此客户端侧重试循环应很短（≤ 2 次）。

---

## 10. 接入清单（零人工参与流程）

AI 必须**严格按顺序**执行以下步骤：

1. **定位代理**
   - 读 `CLIPROXY_BASE_URL`，否则取默认 `http://localhost:8317`。
   - `GET /healthz` 期望 `200 {"status":"ok"}`。失败则代理未运行，
     停止并向用户报错。
2. **获取 API Key**
   - 读 `CLIPROXY_API_KEY`；否则读工程目录下 `.env`。
   - 如果都没有，再尝试 `config.yaml → api-keys:` 的第一项；
     只有**三者都空**时才询问用户。
3. **验证鉴权**
   - 带 `Authorization: Bearer <key>` 调 `GET /v1/models`。
   - 返回 `401` → 停止并询问用户。
4. **选择模型**
   - 在 `/v1/models` 的 `data[].id` 中按任务过滤：
     - 需要工具调用 / 长上下文 → `claude-sonnet-*` 或 `gpt-5`
     - 廉价 / 快速 → `gemini-2.5-flash`、`claude-haiku-*`、`gpt-5-mini`
     - 图像生成 → 支持 `/v1/images/generations` 的模型
5. **注入配置**（写入目标工程）
   - 优先使用环境变量（§8）。
   - 如果工程已有配置文件（`.continuerc`、`mcp.json`、
     Cursor 的 `settings.json` 等），**追加**一个名为 `cliproxy` 的 Provider，
     指向代理的 `baseURL` / `apiKey` / `model`。**不要**删除用户原有的其他 Provider。
6. **冒烟测试**
   - 发一个非流式请求（§6）。HTTP 200 即成功。
7. **输出变更摘要**，便于用户审计。

第 1 / 2 步失败时必须向用户索要缺失信息；其余情况全程无需人工参与。

---

## 11. 值得了解的高级行为

- **模型别名 / 前缀**：运维者可能定义 `test/gpt-5` 这样的别名将请求钉在某个凭据上。
  若模型 ID 中含 `/`，把它当作不透明字符原样透传。
- **会话亲和（session affinity）**：若 `routing.session-affinity: true`，
  代理会按 `X-Session-ID`、`Idempotency-Key`、`metadata.user_id` 或前若干条消息
  的哈希把会话绑定到同一个上游账号。多数接入无需关心。
- **透传自定义头**：客户端发送的自定义头（如 Anthropic beta flags）会被透传，
  除非被 per-provider 过滤器剥离。
- **Thinking / Reasoning**：OpenAI 的 `reasoning.effort`、Anthropic 的
  `thinking`、Gemini 的 `thinkingConfig` 会被代理统一归一化；按 SDK 原生写法传即可。
- **Management API**：`/v0/management/*` 在未配置
  `remote-management.secret-key` 时不可用。AI 接入流程不应触碰它。

---

## 12. 一行验证命令

接入完成后，用以下命令做最终自检：

```bash
curl -fsS -H "Authorization: Bearer $CLIPROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5","messages":[{"role":"user","content":"ping"}]}' \
  "$CLIPROXY_BASE_URL/v1/chat/completions" | jq -r '.choices[0].message.content'
```

若打印出非空回复，则工程已完整接入。
