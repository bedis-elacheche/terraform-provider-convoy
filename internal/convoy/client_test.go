package convoy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL+"/api", "key", WithBackoff(func(int) time.Duration { return time.Millisecond }))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewClientValidation(t *testing.T) {
	if _, err := NewClient("://bad", "k"); err == nil {
		t.Error("expected invalid URL error")
	}
	if _, err := NewClient("", ""); err == nil {
		t.Error("expected missing key error")
	}
	c, err := NewClient("", "k")
	if err != nil || c.baseURL != DefaultBaseURL {
		t.Errorf("default base URL not applied: %v %v", c, err)
	}
}

func TestEnvelopeDecodingAndHeaders(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("missing bearer token")
		}
		if r.URL.Path != "/api/v1/projects/p%2F1/endpoints/e1" && r.URL.RawPath != "/api/v1/projects/p%2F1/endpoints/e1" {
			t.Errorf("path not escaped: %s", r.URL.RawPath)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "message": "ok", "data": map[string]any{
			"uid": "e1", "name": "n", "secrets": []any{
				map[string]any{"value": "old", "expires_at": "2024-01-01T00:00:00Z"},
				map[string]any{"value": "new"},
			}}})
	})
	e, err := c.GetEndpoint(context.Background(), "p/1", "e1")
	if err != nil {
		t.Fatal(err)
	}
	if e.UID != "e1" || e.ActiveSecret() != "new" {
		t.Errorf("unexpected endpoint %+v (active secret %q)", e, e.ActiveSecret())
	}
}

func TestErrorsAndNotFound(t *testing.T) {
	cases := []struct {
		code     int
		msg      string
		notFound bool
	}{
		{404, "endpoint not found", true},
		{400, "failed to find source: not found", true},
		{400, "invalid url", false},
		{401, "unauthorized", false},
	}
	for _, tc := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.code)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": false, "message": tc.msg})
		})
		_, err := c.GetSource(context.Background(), "p", "s")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.code || apiErr.Message != tc.msg {
			t.Errorf("%d: unexpected error %v", tc.code, err)
		}
		if IsNotFound(err) != tc.notFound {
			t.Errorf("%d %q: IsNotFound = %v", tc.code, tc.msg, !tc.notFound)
		}
	}
}

func TestRetries(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "data": map[string]any{"uid": "x"}})
	})
	if _, err := c.GetProject(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestNoRetryOfNonIdempotent5xx(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	})
	if _, err := c.CreateSource(context.Background(), "p", SourceRequest{Name: "x"}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("POST must not be retried on 502, got %d calls", calls)
	}
}

func TestPagination(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{"content": []any{map[string]any{"uid": "a"}}, "pagination": map[string]any{"has_next_page": true, "next_page_cursor": "c1"}}
		if r.URL.Query().Get("next_page_cursor") == "c1" {
			data = map[string]any{"content": []any{map[string]any{"uid": "b"}}, "pagination": map[string]any{"has_next_page": false}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "data": data})
	})
	all, err := c.ListSources(context.Background(), "p")
	if err != nil || len(all) != 2 || all[1].UID != "b" {
		t.Fatalf("unexpected result %+v %v", all, err)
	}
}

func TestSubscriptionIDsFromMetadata(t *testing.T) {
	var s Subscription
	if err := json.Unmarshal([]byte(`{"uid":"s","endpoint_metadata":{"uid":"e"},"source_metadata":{"uid":"src"}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.EndpointID != "e" || s.SourceID != "src" {
		t.Errorf("ids not resolved: %+v", s)
	}
}

func TestAMQPResponseShape(t *testing.T) {
	var a AMQPPubSub
	if err := json.Unmarshal([]byte(`{"host":"h","bindedExchange":"ex","routingKey":"rk"}`), &a); err != nil {
		t.Fatal(err)
	}
	if a.BindExchange == nil || a.BindExchange.Exchange != "ex" || a.BindExchange.RoutingKey != "rk" {
		t.Errorf("binding not folded: %+v", a)
	}
}
