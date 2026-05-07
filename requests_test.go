package requests_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shijl0925/go-requests"
)

// ---- helpers ---------------------------------------------------------------

// newTestServer creates an httptest.Server and returns it together with its
// URL. The caller must call server.Close() when done.
func newTestServer(handler http.Handler) (*httptest.Server, string) {
	srv := httptest.NewServer(handler)
	return srv, srv.URL
}

// echoHandler returns a handler that echoes the request as JSON.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type echoBody struct {
			Method  string            `json:"method"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
			Params  map[string]string `json:"params"`
		}

		params := make(map[string]string)
		for k, vs := range r.URL.Query() {
			params[k] = vs[0]
		}
		headers := make(map[string]string)
		for k, vs := range r.Header {
			headers[k] = vs[0]
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(echoBody{
			Method:  r.Method,
			URL:     r.URL.String(),
			Headers: headers,
			Params:  params,
		})
	})
}

// ---- tests -----------------------------------------------------------------

func TestGetRequest(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	resp, err := requests.Get(baseURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestNilOptionIsIgnored(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	var opt requests.Option
	resp, err := requests.Get(baseURL, opt)
	if err != nil {
		t.Fatalf("Get with nil option failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestDefaultSessionResetClearsSharedCookies(t *testing.T) {
	requests.ResetDefaultSession()
	defer requests.ResetDefaultSession()

	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			http.SetCookie(w, &http.Cookie{Name: "shared", Value: "yes"})
			return
		}
		if ck, err := r.Cookie("shared"); err == nil {
			fmt.Fprint(w, ck.Value)
			return
		}
		fmt.Fprint(w, "none")
	}))
	defer srv.Close()

	if _, err := requests.Get(baseURL + "/set"); err != nil {
		t.Fatalf("set cookie request failed: %v", err)
	}
	resp, err := requests.Get(baseURL + "/echo")
	if err != nil {
		t.Fatalf("echo cookie request failed: %v", err)
	}
	if resp.Text() != "yes" {
		t.Fatalf("expected shared cookie, got %q", resp.Text())
	}

	requests.ResetDefaultSession()
	resp, err = requests.Get(baseURL + "/echo")
	if err != nil {
		t.Fatalf("echo after reset failed: %v", err)
	}
	if resp.Text() != "none" {
		t.Fatalf("expected reset to clear cookie, got %q", resp.Text())
	}
}

func TestGetWithParams(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.Params{"foo": "bar", "baz": "qux"})
	if err != nil {
		t.Fatalf("Get with params failed: %v", err)
	}

	var body map[string]interface{}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}

	paramsRaw, ok := body["params"].(map[string]interface{})
	if !ok {
		t.Fatal("params not found in response body")
	}
	if paramsRaw["foo"] != "bar" {
		t.Errorf("expected foo=bar, got %v", paramsRaw["foo"])
	}
	if paramsRaw["baz"] != "qux" {
		t.Errorf("expected baz=qux, got %v", paramsRaw["baz"])
	}
}

func TestPostJSON(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "expected Content-Type application/json", http.StatusBadRequest)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL, requests.JSON{"hello": "world"})
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := resp.JSON(&result); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if result["hello"] != "world" {
		t.Errorf("expected hello=world, got %v", result["hello"])
	}
}

func TestPostFormData(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "parse form failed", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"username": r.FormValue("username"),
			"password": r.FormValue("password"),
		})
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL, requests.Data{"username": "admin", "password": "secret"})
	if err != nil {
		t.Fatalf("Post form data failed: %v", err)
	}

	var result map[string]string
	if err := resp.JSON(&result); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if result["username"] != "admin" {
		t.Errorf("expected username=admin, got %q", result["username"])
	}
}

func TestPutRequest(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	resp, err := requests.Put(baseURL)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	var body map[string]interface{}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if body["method"] != "PUT" {
		t.Errorf("expected method PUT, got %v", body["method"])
	}
}

func TestPatchRequest(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	resp, err := requests.Patch(baseURL)
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}
	var body map[string]interface{}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if body["method"] != "PATCH" {
		t.Errorf("expected method PATCH, got %v", body["method"])
	}
}

func TestDeleteRequest(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	resp, err := requests.Delete(baseURL)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	var body map[string]interface{}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if body["method"] != "DELETE" {
		t.Errorf("expected method DELETE, got %v", body["method"])
	}
}

func TestHeadRequest(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "head-value")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := requests.Head(baseURL)
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Headers.Get("X-Custom-Header") != "head-value" {
		t.Errorf("expected X-Custom-Header=head-value, got %q", resp.Headers.Get("X-Custom-Header"))
	}
}

func TestCustomHeaders(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.Headers{"X-My-Header": "my-value"})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	var body map[string]interface{}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	headers, ok := body["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("headers not found")
	}
	if headers["X-My-Header"] != "my-value" {
		t.Errorf("expected X-My-Header=my-value, got %v", headers["X-My-Header"])
	}
}

func TestCookies(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "cookie not found", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"session": ck.Value})
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.Cookies{"session": "abc123"})
	if err != nil {
		t.Fatalf("Get with cookies failed: %v", err)
	}
	var result map[string]string
	if err := resp.JSON(&result); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if result["session"] != "abc123" {
		t.Errorf("expected session=abc123, got %q", result["session"])
	}
}

func TestBasicAuth(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.Auth{Provider: requests.BasicAuth{Username: "admin", Password: "secret"}})
	if err != nil {
		t.Fatalf("Get with basic auth failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTokenAuth(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mytoken" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.Auth{Provider: requests.TokenAuth{Token: "mytoken"}})
	if err != nil {
		t.Fatalf("Get with token auth failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDigestAuthClosesInitialStreamResponse(t *testing.T) {
	firstBody := &closeTrackingReadCloser{reader: strings.NewReader("unauthorized")}
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			header := make(http.Header)
			header.Set("WWW-Authenticate", `Digest realm="test", nonce="nonce-value"`)
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     header,
				Body:       firstBody,
				Request:    req,
			}, nil
		}
		if req.Header.Get("Authorization") == "" {
			t.Fatal("expected digest Authorization header on retry")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	resp, err := requests.Get("http://example.test",
		requests.RoundTripper{Transport: rt},
		requests.Stream(true),
		requests.Auth{Provider: requests.DigestAuth{Username: "user", Password: "pass"}},
	)
	if err != nil {
		t.Fatalf("Get with digest auth failed: %v", err)
	}
	defer resp.Close()
	if calls != 2 {
		t.Fatalf("expected digest retry, got %d calls", calls)
	}
	if !firstBody.closed {
		t.Fatal("expected initial 401 response body to be closed before digest retry")
	}
}

func TestResponseOk(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/notfound":
			w.WriteHeader(http.StatusNotFound)
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	tests := []struct {
		path string
		want bool
	}{
		{"/ok", true},
		{"/notfound", false},
		{"/error", false},
	}

	for _, tt := range tests {
		resp, err := requests.Get(baseURL + tt.path)
		if err != nil {
			t.Fatalf("Get %s failed: %v", tt.path, err)
		}
		if resp.Ok() != tt.want {
			t.Errorf("path %s: Ok()=%v, want %v", tt.path, resp.Ok(), tt.want)
		}
	}
}

func TestRaiseForStatus(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	err = resp.RaiseForStatus()
	if err == nil {
		t.Fatal("expected error from RaiseForStatus, got nil")
	}
	var httpErr *requests.HTTPError
	if !errAs(err, &httpErr) {
		t.Fatalf("expected *requests.HTTPError, got %T", err)
	}
}

// errAs is a simple errors.As replacement for use in tests.
func errAs(err error, target **requests.HTTPError) bool {
	if e, ok := err.(*requests.HTTPError); ok {
		*target = e
		return true
	}
	return false
}

func TestResponseText(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello, world")
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.Text() != "hello, world" {
		t.Errorf("expected 'hello, world', got %q", resp.Text())
	}
	// Content() and Text() should both work.
	if string(resp.Content()) != "hello, world" {
		t.Errorf("expected Content()='hello, world', got %q", string(resp.Content()))
	}
}

func TestTimeout(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := requests.Get(baseURL, requests.Timeout(50*time.Millisecond))
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestWithContextFunction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := requests.Get("http://example.test", requests.WithContext(ctx))
	if err == nil {
		t.Fatal("expected canceled context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestAllowRedirectsFalse(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/destination", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.AllowRedirects(false))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 (not following redirect), got %d", resp.StatusCode)
	}
}

func TestAllowRedirectsTrue(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/destination", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.AllowRedirects(true))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (after redirect), got %d", resp.StatusCode)
	}
}

func TestSession(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	s := requests.NewSession()
	s.Headers.Set("X-Session-Header", "persistent")

	for i := 0; i < 3; i++ {
		resp, err := s.Get(baseURL)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		var body map[string]interface{}
		if err := resp.JSON(&body); err != nil {
			t.Fatalf("JSON decode failed: %v", err)
		}
		headers, ok := body["headers"].(map[string]interface{})
		if !ok {
			t.Fatal("headers not found in body")
		}
		if headers["X-Session-Header"] != "persistent" {
			t.Errorf("request %d: expected X-Session-Header=persistent, got %v", i, headers["X-Session-Header"])
		}
	}
}

func TestSessionReusesDefaultTransport(t *testing.T) {
	var newConnections int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt32(&newConnections, 1)
		}
	}
	srv.Start()
	defer srv.Close()

	s := requests.NewSession()
	for i := 0; i < 3; i++ {
		resp, err := s.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if resp.Text() != "ok" {
			t.Fatalf("request %d got body %q", i+1, resp.Text())
		}
	}
	if got := atomic.LoadInt32(&newConnections); got > 2 {
		t.Fatalf("expected at most 2 new connections across 3 requests, got %d", got)
	}
}

func TestSessionMethodsConcurrentUse(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})
	s := requests.NewSession().SetRoundTripper(rt)

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.SetHeader("X-Test", fmt.Sprintf("%d", i)).
				SetAuth(requests.TokenAuth{Token: fmt.Sprintf("token-%d", i)}).
				SetTimeout(time.Second)
		}(i)
		go func() {
			defer wg.Done()
			resp, err := s.Get("http://example.test")
			if err != nil {
				t.Errorf("Get failed: %v", err)
				return
			}
			if resp.Text() != "ok" {
				t.Errorf("unexpected body: %q", resp.Text())
			}
		}()
	}
	wg.Wait()
}

func TestSessionCookiePersistence(t *testing.T) {
	requestCount := 0
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First request: set a cookie.
			http.SetCookie(w, &http.Cookie{Name: "token", Value: "secret"})
			w.WriteHeader(http.StatusOK)
			return
		}
		// Subsequent requests: check for the cookie.
		ck, err := r.Cookie("token")
		if err != nil {
			http.Error(w, "no cookie", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, ck.Value)
	}))
	defer srv.Close()

	s := requests.NewSession()
	if _, err := s.Get(baseURL); err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	resp, err := s.Get(baseURL)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if resp.Text() != "secret" {
		t.Errorf("expected cookie value 'secret', got %q", resp.Text())
	}
}

func TestFileUpload(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, "parse multipart failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("upload")
		if err != nil {
			http.Error(w, "form file error: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		content, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "read file failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"content": string(content)})
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL, requests.Files{
		"upload": requests.FileField{
			FileName: "hello.txt",
			Content:  strings.NewReader("hello"),
		},
	})
	if err != nil {
		t.Fatalf("Post with file failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, resp.Text())
	}
}

func TestJSONBodyStruct(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p payload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p)
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL, requests.JSONBody{Value: payload{Name: "Alice", Age: 30}})
	if err != nil {
		t.Fatalf("Post JSONBody failed: %v", err)
	}
	var result payload
	if err := resp.JSON(&result); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestRawBody(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		fmt.Fprint(w, ct)
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL,
		requests.Body{Reader: strings.NewReader("raw content")},
		requests.Headers{"Content-Type": "text/plain"},
	)
	if err != nil {
		t.Fatalf("Post raw body failed: %v", err)
	}
	if resp.Text() != "text/plain" {
		t.Errorf("expected text/plain, got %q", resp.Text())
	}
}

func TestStreamedMultipartUpload(t *testing.T) {
	serverStarted := make(chan struct{})
	releaseBody := make(chan struct{})

	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(serverStarted)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, "parse multipart failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("upload")
		if err != nil {
			http.Error(w, "form file error: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		content, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "read file failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, string(content))
	}))
	defer srv.Close()

	errCh := make(chan error, 1)
	go func() {
		resp, err := requests.Post(baseURL, requests.Files{
			"upload": requests.FileField{
				FileName: "stream.txt",
				Content: &blockingReader{
					data:    []byte("streamed body"),
					release: releaseBody,
				},
			},
		})
		if err != nil {
			errCh <- err
			return
		}
		if resp.Text() != "streamed body" {
			errCh <- fmt.Errorf("expected streamed body echo, got %q", resp.Text())
			return
		}
		errCh <- nil
	}()

	select {
	case <-serverStarted:
	case <-time.After(time.Second):
		close(releaseBody)
		if err := <-errCh; err != nil {
			t.Logf("request finished after releasing body: %v", err)
		}
		t.Fatal("server did not receive request before upload body was fully available")
	}

	close(releaseBody)
	if err := <-errCh; err != nil {
		t.Fatalf("streamed upload failed: %v", err)
	}
}

func TestStreamedGetResponse(t *testing.T) {
	finishResponse := make(chan struct{})

	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-finishResponse
		_, _ = w.Write([]byte(" world"))
	}))
	defer srv.Close()

	type result struct {
		resp *requests.Response
		err  error
	}
	respCh := make(chan result, 1)
	go func() {
		resp, err := requests.Get(baseURL, requests.Stream(true))
		respCh <- result{resp: resp, err: err}
	}()

	var resp *requests.Response
	select {
	case result := <-respCh:
		if result.err != nil {
			t.Fatalf("streamed GET failed: %v", result.err)
		}
		resp = result.resp
	case <-time.After(time.Second):
		close(finishResponse)
		t.Fatal("streamed GET did not return before the response body completed")
	}
	defer resp.Close()

	buf := make([]byte, len("hello"))
	if _, err := io.ReadFull(resp.Body(), buf); err != nil {
		t.Fatalf("read first streamed chunk failed: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("expected first chunk %q, got %q", "hello", string(buf))
	}

	close(finishResponse)
	rest, err := io.ReadAll(resp.Body())
	if err != nil {
		t.Fatalf("read remaining streamed body failed: %v", err)
	}
	if string(rest) != " world" {
		t.Fatalf("expected remaining body %q, got %q", " world", string(rest))
	}
}

type blockingReader struct {
	data    []byte
	release <-chan struct{}
	sent    bool
}

func (r *blockingReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	<-r.release
	r.sent = true
	return copy(p, r.data), nil
}

func TestResponseIsRedirect(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dest", http.StatusFound)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.AllowRedirects(false))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !resp.IsRedirect() {
		t.Errorf("expected IsRedirect()=true for 302, got false")
	}
}

func TestSessionDefaultTimeout(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := requests.NewSession()
	s.Timeout = 50 * time.Millisecond

	_, err := s.Get(baseURL)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestOptionsRequest(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			http.Error(w, "expected OPTIONS", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	resp, err := requests.Options(baseURL)
	if err != nil {
		t.Fatalf("Options failed: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestResponseCookies(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "lang", Value: "go"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	found := false
	for _, ck := range resp.Cookies {
		if ck.Name == "lang" && ck.Value == "go" {
			found = true
		}
	}
	if !found {
		t.Error("expected cookie lang=go in response")
	}
}

func TestMultipleParamOptions(t *testing.T) {
	srv, baseURL := newTestServer(echoHandler())
	defer srv.Close()

	// Two separate Params options should be merged.
	resp, err := requests.Get(baseURL,
		requests.Params{"a": "1"},
		requests.Params{"b": "2"},
	)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	var body map[string]interface{}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("JSON decode failed: %v", err)
	}
	params := body["params"].(map[string]interface{})
	if params["a"] != "1" || params["b"] != "2" {
		t.Errorf("expected a=1 and b=2 in params, got %v", params)
	}
}

func TestMultiValueParamsAndData(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Encode() != "tag=go&tag=requests" {
			http.Error(w, "unexpected query: "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "parse form failed", http.StatusBadRequest)
			return
		}
		values := r.PostForm["kind"]
		if len(values) != 2 || values[0] != "client" || values[1] != "http" {
			http.Error(w, fmt.Sprintf("unexpected form: %#v", r.PostForm), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL,
		requests.ParamValues{"tag": {"go", "requests"}},
		requests.DataValues{"kind": {"client", "http"}},
	)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, resp.Text())
	}
}

func TestCommonHeaderOptionsAndContentType(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := map[string]string{
			"userAgent":   r.Header.Get("User-Agent"),
			"referer":     r.Header.Get("Referer"),
			"accept":      r.Header.Get("Accept"),
			"contentType": r.Header.Get("Content-Type"),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(got)
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL,
		requests.JSON{"hello": "world"},
		requests.UserAgent("custom-agent"),
		requests.Referer("https://example.com"),
		requests.Accept("application/json"),
		requests.ContentType("application/vnd.example+json"),
	)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	got, err := resp.JSONMap()
	if err != nil {
		t.Fatalf("JSONMap failed: %v", err)
	}
	if got["userAgent"] != "custom-agent" || got["referer"] != "https://example.com" ||
		got["accept"] != "application/json" || got["contentType"] != "application/vnd.example+json" {
		t.Fatalf("unexpected headers: %#v", got)
	}
}

func TestHeadersContentTypeOverridesAutomaticBodyContentType(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("Content-Type"))
	}))
	defer srv.Close()

	resp, err := requests.Post(baseURL,
		requests.JSON{"hello": "world"},
		requests.Headers{"Content-Type": "application/vnd.example+json; charset=utf-8"},
	)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	if resp.Text() != "application/vnd.example+json; charset=utf-8" {
		t.Fatalf("unexpected Content-Type: %q", resp.Text())
	}
}

func TestResponseHelpersSaveToFileAndElapsed(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `["go","requests"]`)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.Elapsed <= 0 {
		t.Fatalf("expected elapsed time to be recorded, got %s", resp.Elapsed)
	}
	values, err := resp.JSONSlice()
	if err != nil {
		t.Fatalf("JSONSlice failed: %v", err)
	}
	if len(values) != 2 || values[0] != "go" || values[1] != "requests" {
		t.Fatalf("unexpected JSON slice: %#v", values)
	}

	path := t.TempDir() + "/response.json"
	if err := resp.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != `["go","requests"]` {
		t.Fatalf("unexpected saved content: %q", content)
	}
}

func TestRetryStatusCode(t *testing.T) {
	attempts := 0
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.Retry{MaxRetries: 2})
	if err != nil {
		t.Fatalf("Get with retry failed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if resp.StatusCode != http.StatusOK || resp.Text() != "ok" {
		t.Fatalf("unexpected response: status=%d body=%q", resp.StatusCode, resp.Text())
	}
}

func TestSessionSetters(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Session") != "yes" {
			http.Error(w, "missing session header", http.StatusBadRequest)
			return
		}
		if _, _, ok := r.BasicAuth(); !ok {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		if _, err := r.Cookie("token"); err != nil {
			http.Error(w, "missing cookie", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	s := requests.NewSession().
		SetHeader("X-Session", "yes").
		SetAuth(requests.BasicAuth{Username: "user", Password: "pass"}).
		SetTimeout(time.Second)
	if err := s.SetCookie(baseURL, "token", "abc"); err != nil {
		t.Fatalf("SetCookie failed: %v", err)
	}
	resp, err := s.Get(baseURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, resp.Text())
	}
}

func TestMaxRedirectsOption(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/one", http.StatusFound)
		case "/one":
			http.Redirect(w, r, "/two", http.StatusFound)
		case "/two":
			fmt.Fprint(w, "done")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.MaxRedirects(1))
	if err != nil {
		t.Fatalf("Get with MaxRedirects failed: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect response after one redirect, got %d", resp.StatusCode)
	}
	if resp.URL.Path != "/one" {
		t.Fatalf("expected to stop at /one, got %s", resp.URL.Path)
	}
}

func TestMaxRedirectsZeroStopsImmediately(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.MaxRedirects(0))
	if err != nil {
		t.Fatalf("Get with MaxRedirects(0) failed: %v", err)
	}
	// MaxRedirects(0) should return the original redirect response without
	// following it to /next.
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected first redirect status, got %d", resp.StatusCode)
	}
	if resp.URL.Path == "/next" {
		t.Fatalf("expected not to follow redirect to /next")
	}
}

func TestMaxRedirectsOverridesSessionRedirectDefault(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/one", http.StatusFound)
		case "/one":
			http.Redirect(w, r, "/two", http.StatusFound)
		case "/two":
			fmt.Fprint(w, "done")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := requests.NewSession()
	s.AllowRedirects = false

	resp, err := s.Get(baseURL, requests.MaxRedirects(1))
	if err != nil {
		t.Fatalf("Get with MaxRedirects failed: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect response after one redirect, got %d", resp.StatusCode)
	}
	if resp.URL.Path != "/one" {
		t.Fatalf("expected to stop at /one, got %s", resp.URL.Path)
	}
}

func TestAllowRedirectsFalseOverridesMaxRedirects(t *testing.T) {
	srv, baseURL := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer srv.Close()

	resp, err := requests.Get(baseURL, requests.MaxRedirects(5), requests.AllowRedirects(false))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	// AllowRedirects(false) should prevent following /next even when
	// MaxRedirects is also provided.
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected first redirect status, got %d", resp.StatusCode)
	}
	if resp.URL.Path == "/next" {
		t.Fatalf("expected not to follow redirect to /next")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type closeTrackingReadCloser struct {
	reader io.Reader
	closed bool
}

func (c *closeTrackingReadCloser) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *closeTrackingReadCloser) Close() error {
	c.closed = true
	return nil
}

func TestCustomHTTPClientAndRoundTripper(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "example.test" {
			t.Fatalf("unexpected host: %s", req.URL.Host)
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Status:     "202 Accepted",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("custom client")),
			Request:    req,
		}, nil
	})
	client := &http.Client{Transport: rt}

	resp, err := requests.Get("http://example.test", requests.HTTPClient{Client: client})
	if err != nil {
		t.Fatalf("Get with custom client failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted || resp.Text() != "custom client" {
		t.Fatalf("unexpected custom client response: status=%d body=%q", resp.StatusCode, resp.Text())
	}

	resp, err = requests.Get("http://example.test", requests.RoundTripper{Transport: rt})
	if err != nil {
		t.Fatalf("Get with custom round tripper failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected two custom transport calls, got %d", calls)
	}

	s := requests.NewSession().SetClient(client)
	resp, err = s.Get("http://example.test")
	if err != nil {
		t.Fatalf("Get with session client failed: %v", err)
	}
	if calls != 3 || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected session client result: calls=%d status=%d", calls, resp.StatusCode)
	}
}

func TestHTTPErrorManualResponseIsRobust(t *testing.T) {
	err := (&requests.HTTPError{Response: &requests.Response{Status: "500 Internal Server Error"}}).Error()
	if !strings.Contains(err, "500 Internal Server Error") {
		t.Fatalf("unexpected error string: %q", err)
	}
	if got := (&requests.HTTPError{}).Error(); !strings.Contains(got, "response is nil") {
		t.Fatalf("unexpected nil response error string: %q", got)
	}
}

func TestSessionRoundTripperAndTransportConfig(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusCreated,
			Status:     "201 Created",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("session transport")),
			Request:    req,
		}, nil
	})

	s := requests.NewSession().
		SetTransportConfig(requests.TransportConfig{
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     time.Minute,
		}).
		SetRoundTripper(rt)
	resp, err := s.Get("http://example.test")
	if err != nil {
		t.Fatalf("Get with session round tripper failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one round trip, got %d", calls)
	}
	if resp.StatusCode != http.StatusCreated || resp.Text() != "session transport" {
		t.Fatalf("unexpected session round tripper response: status=%d body=%q", resp.StatusCode, resp.Text())
	}
}
