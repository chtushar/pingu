package trace

import (
	"context"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds tracing configuration.
type Config struct {
	Endpoint string // host:port of the OTLP endpoint
	URLPath  string // path for the OTLP traces endpoint
	APIKey   string // API key sent as Authorization header
}

// otelErrorHandler logs OTel internal errors via slog.
type otelErrorHandler struct{}

func (otelErrorHandler) Handle(err error) {
	slog.Error("otel error", "error", err)
}

func Init(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	otel.SetErrorHandler(otelErrorHandler{})

	opts := []otlptracehttp.Option{
		otlptracehttp.WithInsecure(),
	}
	if cfg.Endpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
	}
	if cfg.URLPath != "" {
		opts = append(opts, otlptracehttp.WithURLPath(cfg.URLPath))
	}
	if cfg.APIKey != "" {
		opts = append(opts, otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.APIKey,
		}))
	}

	opts = append(opts, otlptracehttp.WithHTTPClient(&http.Client{
		Transport: &loggingTransport{inner: http.DefaultTransport},
	}))

	slog.Debug("otlp exporter config", "endpoint", cfg.Endpoint, "url_path", cfg.URLPath, "has_api_key", cfg.APIKey != "")

	inner, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	exporter := &loggingExporter{inner: inner}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("pingu")),
	)
	if err != nil {
		return nil, err
	}

	// Use SimpleSpanProcessor for synchronous export (surfaces errors immediately).
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// loggingTransport wraps an http.RoundTripper and logs each request/response.
type loggingTransport struct {
	inner http.RoundTripper
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	slog.Debug("otlp http request",
		"method", req.Method,
		"url", req.URL.String(),
		"content_type", req.Header.Get("Content-Type"),
		"content_length", req.ContentLength,
	)
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		slog.Error("otlp http error", "error", err)
		return resp, err
	}
	slog.Debug("otlp http response", "status", resp.StatusCode, "url", req.URL.String())
	return resp, nil
}

// loggingExporter wraps a SpanExporter and logs each export call.
type loggingExporter struct {
	inner sdktrace.SpanExporter
}

func (e *loggingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name()
	}
	slog.Debug("otlp exporting spans", "count", len(spans), "names", names)
	err := e.inner.ExportSpans(ctx, spans)
	if err != nil {
		slog.Error("otlp export failed", "error", err)
	} else {
		slog.Debug("otlp export ok", "count", len(spans))
	}
	return err
}

func (e *loggingExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

// Tracer returns the pingu tracer.
func Tracer() trace.Tracer {
	return otel.Tracer("pingu")
}
