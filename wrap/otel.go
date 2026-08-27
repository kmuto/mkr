package wrap

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/mackerelio/mkr/logger"
)

func otelEnabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != ""
}

type otelLogExporter struct {
	provider *sdklog.LoggerProvider
	logger   otellog.Logger
}

func newOTelLogExporter(ctx context.Context, hostID string) (*otelLogExporter, error) {
	exporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", "mkr-wrap"),
			attribute.String("host.id", hostID),
		),
	)
	if err != nil {
		return nil, err
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	return &otelLogExporter{
		provider: provider,
		logger:   provider.Logger("mkr-wrap"),
	}, nil
}

func (e *otelLogExporter) shutdown(ctx context.Context) error {
	return e.provider.Shutdown(ctx)
}

func (e *otelLogExporter) newLineWriter(inner io.Writer, severity otellog.Severity, attrs ...attribute.KeyValue) *otelLineWriter {
	return &otelLineWriter{
		inner:    inner,
		logger:   e.logger,
		severity: severity,
		attrs:    attrs,
	}
}

func (e *otelLogExporter) emitSummary(re *result, checkName string) {
	severity := otellog.SeverityInfo
	if !re.Success {
		severity = otellog.SeverityError
	}
	var record otellog.Record
	record.SetTimestamp(time.Now())
	record.SetSeverity(severity)
	record.SetBody(attribute.StringValue(re.Msg))
	record.AddAttributes(
		attribute.String("command", strings.Join(re.Cmd, " ")),
		attribute.String("check_name", checkName),
		attribute.Int("exit_code", re.ExitCode),
		attribute.Bool("success", re.Success),
	)
	e.logger.Emit(context.Background(), record)
}

func (wr *wrap) setupOTel() func(re *result) {
	if !otelEnabled() {
		return nil
	}

	ctx := context.Background()
	checkName := (&result{Cmd: wr.cmd, Name: wr.name}).checkName()
	exp, err := newOTelLogExporter(ctx, wr.hostID)
	if err != nil {
		logger.Logf("warning", "[mkr wrap] failed to initialize OpenTelemetry: %s", err)
		return nil
	}

	cmdStr := strings.Join(wr.cmd, " ")
	baseAttrs := []attribute.KeyValue{
		attribute.String("command", cmdStr),
		attribute.String("check_name", checkName),
	}

	outW := exp.newLineWriter(wr.outStream, otellog.SeverityInfo,
		append(baseAttrs, attribute.String("stream", "stdout"))...,
	)
	errW := exp.newLineWriter(wr.errStream, otellog.SeverityWarn,
		append(baseAttrs, attribute.String("stream", "stderr"))...,
	)

	wr.outStream = outW
	wr.errStream = errW

	return func(re *result) {
		outW.flush()
		errW.flush()
		exp.emitSummary(re, checkName)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := exp.shutdown(shutdownCtx); err != nil {
			logger.Logf("warning", "[mkr wrap] failed to shutdown OpenTelemetry: %s", err)
		}
	}
}

type otelLineWriter struct {
	inner    io.Writer
	buf      []byte
	logger   otellog.Logger
	severity otellog.Severity
	attrs    []attribute.KeyValue
}

func (w *otelLineWriter) Write(p []byte) (int, error) {
	n, err := w.inner.Write(p)
	if n > 0 {
		w.buf = append(w.buf, p[:n]...)
		w.emitLines()
	}
	return n, err
}

func (w *otelLineWriter) emitLines() {
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		w.emit(line)
	}
}

func (w *otelLineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}

func (w *otelLineWriter) emit(line string) {
	var record otellog.Record
	record.SetTimestamp(time.Now())
	record.SetBody(attribute.StringValue(line))
	record.SetSeverity(w.severity)
	record.AddAttributes(w.attrs...)
	w.logger.Emit(context.Background(), record)
}
