package requests_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shijl0925/go-requests"
)

func benchmarkServer() *httptest.Server {
	payload := bytes.Repeat([]byte("x"), 64*1024)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
		case "/large":
			_, _ = w.Write(payload)
		case "/upload":
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte("ok"))
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
}

func BenchmarkNetHTTPGet(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	client := srv.Client()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func BenchmarkRequestsGetDefault(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := requests.Get(srv.URL)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

func BenchmarkNewSessionGet(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewSession()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Get(srv.URL)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

func BenchmarkNewFastSessionGet(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewFastSession()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Get(srv.URL)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body())
		_ = resp.Close()
	}
}

func BenchmarkGetWithParams(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewSession()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Get(srv.URL, requests.Params{"q": "go", "page": "1"})
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

func BenchmarkPostJSON(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewSession()
	body := requests.JSON{"name": "alice", "active": true}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Post(srv.URL+"/json", body)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

func BenchmarkPostForm(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewSession()
	body := requests.Data{"name": "alice", "active": "true"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Post(srv.URL, body)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

func BenchmarkLargeResponseDefault(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewSession()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Get(srv.URL + "/large")
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

func BenchmarkLargeResponseStreaming(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewFastSession()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Get(srv.URL + "/large")
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body())
		_ = resp.Close()
	}
}

func BenchmarkFileUpload(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewSession()
	payload := bytes.Repeat([]byte("x"), 32*1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := s.Post(srv.URL+"/upload", requests.Files{
			"file": {FileName: "bench.txt", Content: bytes.NewReader(payload)},
		})
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Close()
	}
}

func BenchmarkFastSessionParallel(b *testing.B) {
	srv := benchmarkServer()
	defer srv.Close()
	s := requests.NewFastSession()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := s.Get(srv.URL)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body())
			_ = resp.Close()
		}
	})
}
