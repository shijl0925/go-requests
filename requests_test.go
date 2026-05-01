package requests_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
