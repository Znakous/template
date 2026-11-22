package push

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRequestPositionUsesPushContract(t *testing.T) {
	driverID := uuid.New()
	tripID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/push" {
			t.Errorf("request = %s %s, want POST /api/v1/push", r.Method, r.URL.Path)
		}
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.RequestID == "" || body.RecipientID != driverID.String() ||
			body.Kind != "REQUEST_POSITION" || body.Data["trip_id"] != tripID.String() {
			t.Errorf("request body = %+v", body)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.RequestPosition(context.Background(), driverID, tripID); err != nil {
		t.Fatal(err)
	}
}

func TestRequestPositionClassifiesHTTPFailures(t *testing.T) {
	tests := []struct {
		status int
		want   any
	}{
		{status: http.StatusBadRequest, want: (*ClientError)(nil)},
		{status: http.StatusServiceUnavailable, want: (*ServerError)(nil)},
		{status: http.StatusOK, want: (*HTTPError)(nil)},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			err = client.RequestPosition(context.Background(), uuid.New(), uuid.New())
			if err == nil {
				t.Fatal("expected an error")
			}
			switch tt.want.(type) {
			case *ClientError:
				var responseErr *ClientError
				if !errors.As(err, &responseErr) || responseErr.StatusCode != tt.status {
					t.Fatalf("error = %T %v, want ClientError with status %d", err, err, tt.status)
				}
			case *ServerError:
				var responseErr *ServerError
				if !errors.As(err, &responseErr) || responseErr.StatusCode != tt.status {
					t.Fatalf("error = %T %v, want ServerError with status %d", err, err, tt.status)
				}
			case *HTTPError:
				var responseErr *HTTPError
				if !errors.As(err, &responseErr) || responseErr.StatusCode != tt.status {
					t.Fatalf("error = %T %v, want HTTPError with status %d", err, err, tt.status)
				}
			}
		})
	}
}

func TestNewClientRejectsInvalidURL(t *testing.T) {
	if _, err := NewClient("localhost:8090", time.Second); err == nil {
		t.Fatal("expected invalid URL error")
	}
}
