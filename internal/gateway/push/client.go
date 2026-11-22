package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type ClientError struct {
	StatusCode int
}

func (e *ClientError) Error() string {
	return fmt.Sprintf("push service rejected request with HTTP %d", e.StatusCode)
}

type ServerError struct {
	StatusCode int
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("push service failed with HTTP %d", e.StatusCode)
}

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("push service returned unexpected HTTP %d", e.StatusCode)
}

type request struct {
	RequestID   string            `json:"request_id"`
	RecipientID string            `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}

type Client struct {
	url    string
	client *http.Client
}

func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("PUSH_HTTP_URL must be an absolute HTTP URL")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("push request timeout must be positive")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v1/push"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return &Client{
		url: parsed.String(),
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	ctx, span := otel.Tracer("trip-service").Start(ctx, "push.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("http.request.method", http.MethodPost)),
	)
	defer span.End()

	body, err := json.Marshal(request{
		RequestID:   uuid.NewString(),
		RecipientID: driverID.String(),
		Kind:        "REQUEST_POSITION",
		Data:        map[string]string{"trip_id": tripID.String()},
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("encode push request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("create push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("send push request: %w", err)
	}
	span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
	defer response.Body.Close()
	_, readErr := io.Copy(io.Discard, response.Body)
	if readErr != nil {
		span.RecordError(readErr)
		span.SetStatus(codes.Error, readErr.Error())
		return fmt.Errorf("read push response: %w", readErr)
	}
	if response.StatusCode != http.StatusAccepted {
		var err error
		switch {
		case response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError:
			err = &ClientError{StatusCode: response.StatusCode}
		case response.StatusCode >= http.StatusInternalServerError:
			err = &ServerError{StatusCode: response.StatusCode}
		default:
			err = &HTTPError{StatusCode: response.StatusCode}
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (c *Client) Close() {
	c.client.CloseIdleConnections()
}
