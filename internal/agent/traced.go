package agent

import (
	"context"
	"log/slog"
	"pingu/internal/trace"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type tracedTool struct {
	Tool
}

func withTrace(t Tool) Tool {
	return &tracedTool{Tool: t}
}

func (t *tracedTool) Execute(ctx context.Context, input string) (string, error) {
	ctx, span := trace.Tracer().Start(ctx, t.Name(),
		oteltrace.WithAttributes(
			attribute.String("openai.agents.span_type", "function"),
			attribute.String("gen_ai.tool.name", t.Name()),
			attribute.String("gen_ai.tool.input", input),
		),
	)
	defer span.End()

	sc := span.SpanContext()
	slog.Debug("tool span started", "tool", t.Name(), "trace_id", sc.TraceID(), "span_id", sc.SpanID())

	result, err := t.Tool.Execute(ctx, input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return result, err
	}

	span.SetAttributes(attribute.Int("gen_ai.tool.output_length", len(result)))
	return result, nil
}

// SetEmit forwards to the inner tool if it implements EmitSetter.
func (t *tracedTool) SetEmit(emit func(Event)) {
	if es, ok := t.Tool.(EmitSetter); ok {
		es.SetEmit(emit)
	}
}
