package requests_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shijl0925/go-requests"
)

func TestNewFastSessionStreamsByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	s := requests.NewFastSession()
	resp, err := s.Get(srv.URL)
	if err != nil {
		t.Fatalf("fast session request failed: %v", err)
	}
	defer resp.Close()

	body, err := io.ReadAll(resp.Body())
	if err != nil {
		t.Fatalf("read streamed body: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("expected streamed body hello, got %q", string(body))
	}
}

func TestStreamOptionOverridesFastSessionDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	s := requests.NewFastSession()
	resp, err := s.Get(srv.URL, requests.Stream(false))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.Text() != "hello" {
		t.Fatalf("expected cached body hello, got %q", resp.Text())
	}
}

func TestMaxResponseBodySize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer srv.Close()

	_, err := requests.Get(srv.URL, requests.MaxResponseBodySize(3))
	if err == nil || !strings.Contains(err.Error(), "response body exceeds max size") {
		t.Fatalf("expected response size error, got %v", err)
	}

	resp, err := requests.Get(srv.URL, requests.MaxResponseBodySize(6))
	if err != nil {
		t.Fatalf("expected exact-size response to succeed: %v", err)
	}
	if got := resp.Text(); got != "abcdef" {
		t.Fatalf("expected abcdef, got %q", got)
	}
}

func TestTransportConfigDisableCompression(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept-Encoding"); got != "" {
			t.Fatalf("expected no automatic Accept-Encoding, got %q", got)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	s := requests.NewSession().SetTransportConfig(requests.TransportConfig{DisableCompression: true})
	resp, err := s.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.Text() != "ok" {
		t.Fatalf("expected ok, got %q", resp.Text())
	}
}

func TestPerformanceTransportConfig(t *testing.T) {
	cfg := requests.PerformanceTransportConfig()
	if cfg.MaxIdleConns < 100 || cfg.MaxIdleConnsPerHost < 10 {
		t.Fatalf("performance config should enlarge idle connection pools: %+v", cfg)
	}
	if !cfg.ForceAttemptHTTP2 {
		t.Fatalf("performance config should keep HTTP/2 enabled: %+v", cfg)
	}
}
