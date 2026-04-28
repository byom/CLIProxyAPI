package modeltest

import (
	"strings"
)

// DoneSentinel is the payload OpenAI-style servers emit to mark the end of a stream.
const DoneSentinel = "[DONE]"

// SSEEvent is a single dispatched Server-Sent-Events record.
type SSEEvent struct {
	Event string
	Data  string
}

// IsDone reports whether the event is the terminal [DONE] sentinel.
func (e SSEEvent) IsDone() bool {
	return strings.TrimSpace(e.Data) == DoneSentinel
}

// SSEParser incrementally decodes SSE text. It tolerates CRLF / bare-LF line
// endings, `event:` lines, multi-line `data:` payloads and comment lines
// that start with `:`.
type SSEParser struct {
	buffer    strings.Builder
	eventName string
	dataLines []string
	pending   bool
}

// NewSSEParser returns a fresh parser.
func NewSSEParser() *SSEParser {
	return &SSEParser{}
}

// Feed consumes another text chunk and returns any events that completed
// within this call. It is safe to call repeatedly.
func (p *SSEParser) Feed(chunk string) []SSEEvent {
	normalized := strings.ReplaceAll(chunk, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	p.buffer.WriteString(normalized)
	out := []SSEEvent{}

	for {
		buf := p.buffer.String()
		idx := strings.IndexByte(buf, '\n')
		if idx < 0 {
			break
		}
		line := buf[:idx]
		p.buffer.Reset()
		p.buffer.WriteString(buf[idx+1:])
		if ev, ok := p.handleLine(line); ok {
			out = append(out, ev)
		}
	}
	return out
}

// Close flushes any pending event at stream end and returns it.
func (p *SSEParser) Close() []SSEEvent {
	out := []SSEEvent{}
	if p.buffer.Len() > 0 {
		line := p.buffer.String()
		p.buffer.Reset()
		if ev, ok := p.handleLine(line); ok {
			out = append(out, ev)
		}
	}
	if p.pending {
		out = append(out, p.dispatch())
	}
	return out
}

func (p *SSEParser) handleLine(line string) (SSEEvent, bool) {
	if line == "" {
		if p.pending {
			return p.dispatch(), true
		}
		return SSEEvent{}, false
	}
	if strings.HasPrefix(line, ":") {
		return SSEEvent{}, false
	}
	field, value, _ := strings.Cut(line, ":")
	value = strings.TrimPrefix(value, " ")
	switch field {
	case "event":
		p.eventName = value
		p.pending = true
	case "data":
		p.dataLines = append(p.dataLines, value)
		p.pending = true
	}
	return SSEEvent{}, false
}

func (p *SSEParser) dispatch() SSEEvent {
	ev := SSEEvent{Event: p.eventName, Data: strings.Join(p.dataLines, "\n")}
	p.eventName = ""
	p.dataLines = nil
	p.pending = false
	return ev
}
