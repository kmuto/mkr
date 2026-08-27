package wrap

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/embedded"
)

type capturedRecord struct {
	Body     string
	Severity otellog.Severity
	Attrs    map[string]string
}

type mockLogger struct {
	embedded.Logger
	mu      sync.Mutex
	records []capturedRecord
}

func (l *mockLogger) Emit(_ context.Context, r otellog.Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	attrs := make(map[string]string)
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value.AsString()
		return true
	})
	l.records = append(l.records, capturedRecord{
		Body:     r.Body().AsString(),
		Severity: r.Severity(),
		Attrs:    attrs,
	})
}

func (l *mockLogger) Enabled(_ context.Context, _ otellog.EnabledParameters) bool {
	return true
}

func TestOTelLineWriter_Lines(t *testing.T) {
	ml := &mockLogger{}
	var inner bytes.Buffer
	w := &otelLineWriter{
		inner:    &inner,
		logger:   ml,
		severity: otellog.SeverityInfo,
		attrs: []attribute.KeyValue{
			attribute.String("stream", "stdout"),
		},
	}

	w.Write([]byte("hello\nworld\n"))

	if len(ml.records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(ml.records))
	}
	if ml.records[0].Body != "hello" {
		t.Errorf("expected body 'hello', got %q", ml.records[0].Body)
	}
	if ml.records[1].Body != "world" {
		t.Errorf("expected body 'world', got %q", ml.records[1].Body)
	}
	if ml.records[0].Severity != otellog.SeverityInfo {
		t.Errorf("expected SeverityInfo, got %v", ml.records[0].Severity)
	}
	if ml.records[0].Attrs["stream"] != "stdout" {
		t.Errorf("expected stream=stdout, got %q", ml.records[0].Attrs["stream"])
	}
	if inner.String() != "hello\nworld\n" {
		t.Errorf("inner writer got %q", inner.String())
	}
}

func TestOTelLineWriter_PartialLines(t *testing.T) {
	ml := &mockLogger{}
	var inner bytes.Buffer
	w := &otelLineWriter{
		inner:    &inner,
		logger:   ml,
		severity: otellog.SeverityWarn,
	}

	w.Write([]byte("hel"))
	w.Write([]byte("lo\nwor"))
	if len(ml.records) != 1 {
		t.Fatalf("expected 1 record after partial writes, got %d", len(ml.records))
	}
	if ml.records[0].Body != "hello" {
		t.Errorf("expected body 'hello', got %q", ml.records[0].Body)
	}

	w.flush()
	if len(ml.records) != 2 {
		t.Fatalf("expected 2 records after flush, got %d", len(ml.records))
	}
	if ml.records[1].Body != "wor" {
		t.Errorf("expected body 'wor', got %q", ml.records[1].Body)
	}
}

func TestOTelLineWriter_FlushEmpty(t *testing.T) {
	ml := &mockLogger{}
	var inner bytes.Buffer
	w := &otelLineWriter{
		inner:    &inner,
		logger:   ml,
		severity: otellog.SeverityInfo,
	}

	w.flush()
	if len(ml.records) != 0 {
		t.Errorf("expected 0 records on empty flush, got %d", len(ml.records))
	}
}

func TestOTelEnabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	if otelEnabled() {
		t.Error("expected otelEnabled() to return false when env vars are empty")
	}

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	if !otelEnabled() {
		t.Error("expected otelEnabled() to return true when OTEL_EXPORTER_OTLP_ENDPOINT is set")
	}

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "http://localhost:4318")
	if !otelEnabled() {
		t.Error("expected otelEnabled() to return true when OTEL_EXPORTER_OTLP_LOGS_ENDPOINT is set")
	}
}
