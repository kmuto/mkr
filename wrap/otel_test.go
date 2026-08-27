package wrap

import (
	"bytes"
	"context"
	"os"
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

func TestEnvTruthy(t *testing.T) {
	key := "TEST_TRUTHY_VAR"
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"no", false},
		{"1", true},
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"yes", true},
		{"Yes", true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv(key, tt.value)
			t.Cleanup(func() { os.Unsetenv(key) })
			if got := envTruthy(key); got != tt.want {
				t.Errorf("envTruthy(%q) with value %q = %v, want %v", key, tt.value, got, tt.want)
			}
		})
	}
}

func TestSetupOTel_Disabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("MKR_WRAP_OTEL_LOG", "")

	wr := &wrap{cmd: []string{"echo", "hi"}, outStream: os.Stdout, errStream: os.Stderr}
	if cleanup := wr.setupOTel(); cleanup != nil {
		t.Error("expected setupOTel to return nil when no env vars are set")
	}
}

func TestSetupOTel_MackerelDirect_NoAPIKey(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("MKR_WRAP_OTEL_LOG", "1")

	wr := &wrap{cmd: []string{"echo", "hi"}, apikey: "", outStream: os.Stdout, errStream: os.Stderr}
	if cleanup := wr.setupOTel(); cleanup != nil {
		t.Error("expected setupOTel to return nil when MKR_WRAP_OTEL_LOG is set but apikey is empty")
	}
}
