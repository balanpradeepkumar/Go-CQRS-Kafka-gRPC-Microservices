package tracing

import (
	"context"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc/metadata"
	"strings"
)

func StartHttpServerTracerSpan(c echo.Context, operationName string) (context.Context, opentracing.Span) {
	spanCtx, err := opentracing.GlobalTracer().Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(c.Request().Header))
	if err != nil {
		serverSpan := opentracing.GlobalTracer().StartSpan(operationName)
		ctx := opentracing.ContextWithSpan(c.Request().Context(), serverSpan)
		return ctx, serverSpan
	}

	serverSpan := opentracing.GlobalTracer().StartSpan(operationName, ext.RPCServerOption(spanCtx))
	ctx := opentracing.ContextWithSpan(c.Request().Context(), serverSpan)

	return ctx, serverSpan
}

func GetTextMapCarrierFromMetaData(ctx context.Context) opentracing.TextMapCarrier {
	metadataMap := make(opentracing.TextMapCarrier)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for key, _ := range md.Copy() {
			metadataMap.Set(key, md.Get(key)[0])
		}
	}
	return metadataMap
}

func StartGrpcServerTracerSpan(ctx context.Context, operationName string) (context.Context, opentracing.Span) {
	textMapCarrierFromMetaData := GetTextMapCarrierFromMetaData(ctx)

	span, err := opentracing.GlobalTracer().Extract(opentracing.TextMap, textMapCarrierFromMetaData)
	if err != nil {
		serverSpan := opentracing.GlobalTracer().StartSpan(operationName)
		ctx = opentracing.ContextWithSpan(ctx, serverSpan)
		return ctx, serverSpan
	}

	serverSpan := opentracing.GlobalTracer().StartSpan(operationName, ext.RPCServerOption(span))
	ctx = opentracing.ContextWithSpan(ctx, serverSpan)

	return ctx, serverSpan
}

func StartKafkaConsumerTracerSpan(ctx context.Context, headers []kafka.Header, operationName string) (context.Context, opentracing.Span) {
	carrierFromKafkaHeaders := TextMapCarrierFromKafkaMessageHeaders(headers)

	spanCtx, err := opentracing.GlobalTracer().Extract(opentracing.TextMap, carrierFromKafkaHeaders)
	if err != nil {
		serverSpan := opentracing.GlobalTracer().StartSpan(operationName)
		ctx = opentracing.ContextWithSpan(ctx, serverSpan)
		return ctx, serverSpan
	}

	serverSpan := opentracing.GlobalTracer().StartSpan(operationName, ext.RPCServerOption(spanCtx))
	ctx = opentracing.ContextWithSpan(ctx, serverSpan)

	return ctx, serverSpan
}

func TextMapCarrierToBinary(textMap opentracing.TextMapCarrier) ([]byte, error) {
	return json.Marshal(textMap)
}

func TextMapCarrierFromBinary(textMapBytes []byte) (opentracing.TextMapCarrier, error) {
	m := make(map[string]string)
	if err := json.Unmarshal(textMapBytes, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func TextMapCarrierFromKafkaHeaders(headers []kafka.Header) (opentracing.TextMapCarrier, error) {
	for _, header := range headers {
		if strings.EqualFold(header.Key, "tracing") {
			carrierFromBinary, err := TextMapCarrierFromBinary(header.Value)
			if err != nil {
				return nil, err
			}
			return carrierFromBinary, nil
		}
	}
	return nil, errors.New("no tracing headers provided")
}

func TextMapCarrierToKafkaMessageHeaders(textMap opentracing.TextMapCarrier) []kafka.Header {
	headers := make([]kafka.Header, 0, len(textMap))

	if err := textMap.ForeachKey(func(key, val string) error {
		headers = append(headers, kafka.Header{
			Key:   key,
			Value: []byte(val),
		})
		return nil
	}); err != nil {
		return headers
	}
	return headers
}

func TextMapCarrierFromKafkaMessageHeaders(headers []kafka.Header) opentracing.TextMapCarrier {
	textMap := make(map[string]string, len(headers))
	for _, header := range headers {
		textMap[header.Key] = string(header.Value)
	}
	return opentracing.TextMapCarrier(textMap)
}

func InjectTextMapCarrier(spanCtx opentracing.SpanContext) (opentracing.TextMapCarrier, error) {
	m := make(opentracing.TextMapCarrier)
	if err := opentracing.GlobalTracer().Inject(spanCtx, opentracing.TextMap, m); err != nil {
		return nil, err
	}
	return m, nil
}

func InjectTextMapCarrierToGrpcMetaData(ctx context.Context, spanCtx opentracing.SpanContext) context.Context {
	if textMapCarrier, err := InjectTextMapCarrier(spanCtx); err == nil {
		md := metadata.New(textMapCarrier)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}
