package modeltest

import "testing"

func collectEvents(chunks []string) []SSEEvent {
	p := NewSSEParser()
	var out []SSEEvent
	for _, chunk := range chunks {
		out = append(out, p.Feed(chunk)...)
	}
	out = append(out, p.Close()...)
	return out
}

func TestSSESingleEventDispatchedAfterBlankLine(t *testing.T) {
	evs := collectEvents([]string{"data: hello\n\n"})
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if evs[0].Data != "hello" {
		t.Fatalf("data mismatch: %q", evs[0].Data)
	}
	if evs[0].IsDone() {
		t.Fatal("hello should not look like [DONE]")
	}
}

func TestSSEEventFieldAndMultiDataLines(t *testing.T) {
	evs := collectEvents([]string{
		"event: response.delta\n",
		"data: line-one\n",
		"data: line-two\n",
		"\n",
	})
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if evs[0].Event != "response.delta" {
		t.Fatalf("event mismatch: %q", evs[0].Event)
	}
	if evs[0].Data != "line-one\nline-two" {
		t.Fatalf("data mismatch: %q", evs[0].Data)
	}
}

func TestSSEDoneSentinelDetected(t *testing.T) {
	evs := collectEvents([]string{"data: [DONE]\n\n"})
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if !evs[0].IsDone() {
		t.Fatal("expected [DONE] to be detected")
	}
}

func TestSSESplitChunkBoundariesAreHandled(t *testing.T) {
	evs := collectEvents([]string{"data: par", "tial-", "payload\n", "\n"})
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if evs[0].Data != "partial-payload" {
		t.Fatalf("data mismatch: %q", evs[0].Data)
	}
}

func TestSSECRLFLineEndings(t *testing.T) {
	evs := collectEvents([]string{"event: x\r\ndata: y\r\n\r\n"})
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if evs[0].Event != "x" || evs[0].Data != "y" {
		t.Fatalf("unexpected event: %+v", evs[0])
	}
}

func TestSSECommentLinesIgnored(t *testing.T) {
	evs := collectEvents([]string{": heartbeat\n\n", "data: hi\n\n"})
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if evs[0].Data != "hi" {
		t.Fatalf("data mismatch: %q", evs[0].Data)
	}
}

func TestSSEUnterminatedEventFlushedOnClose(t *testing.T) {
	p := NewSSEParser()
	_ = p.Feed("data: tail-without-newline")
	flushed := p.Close()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flushed event, got %d", len(flushed))
	}
	if flushed[0].Data != "tail-without-newline" {
		t.Fatalf("data mismatch: %q", flushed[0].Data)
	}
}
