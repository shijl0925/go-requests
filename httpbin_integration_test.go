//go:build integration

package requests_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shijl0925/go-requests"
)

const defaultHTTPBinURL = "https://httpbin.org"

var (
	httpbinCheckOnce sync.Once
	httpbinCheckErr  error
)

func httpbinURL(path string) string {
	base := os.Getenv("HTTPBIN_BASE_URL")
	if base == "" {
		base = defaultHTTPBinURL
	}
	return strings.TrimRight(base, "/") + path
}

func requireHTTPBinAvailable(t *testing.T) {
	t.Helper()

	httpbinCheckOnce.Do(func() {
		client := &http.Client{Timeout: 5 * time.Second}
		req, err := http.NewRequest(http.MethodHead, httpbinURL("/get"), nil)
		if err != nil {
			httpbinCheckErr = err
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			httpbinCheckErr = err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			httpbinCheckErr = &requests.HTTPError{Response: &requests.Response{StatusCode: resp.StatusCode, Status: resp.Status, URL: resp.Request.URL}}
		}
	})

	if httpbinCheckErr != nil {
		t.Skipf("skipping httpbin integration test: %s is unavailable: %v", httpbinURL(""), httpbinCheckErr)
	}
}

func decodeHTTPBinJSON(t *testing.T, resp *requests.Response) map[string]interface{} {
	t.Helper()

	var body map[string]interface{}
	if err := resp.JSON(&body); err != nil {
		t.Fatalf("failed to decode httpbin JSON response: %v\nstatus=%d body=%s", err, resp.StatusCode, resp.Text())
	}
	return body
}

func requireStatus(t *testing.T, resp *requests.Response, status int) {
	t.Helper()

	if resp.StatusCode != status {
		t.Fatalf("expected status %d, got %d: %s", status, resp.StatusCode, resp.Text())
	}
}

func TestHTTPBinIntegrationGetParamsHeadersAndResponseHelpers(t *testing.T) {
	requireHTTPBinAvailable(t)

	resp, err := requests.Get(httpbinURL("/get"),
		requests.Params{"foo": "bar", "lang": "go"},
		requests.Headers{"X-Go-Requests-Test": "headers"},
		requests.Timeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("GET /get failed: %v", err)
	}
	requireStatus(t, resp, http.StatusOK)

	if !resp.Ok() {
		t.Fatal("expected Ok() to be true for HTTP 200")
	}
	if resp.Encoding() == "" {
		t.Fatal("expected response encoding to be populated")
	}
	if len(resp.Content()) == 0 || resp.Text() == "" {
		t.Fatal("expected response body to be readable via Content and Text")
	}

	body := decodeHTTPBinJSON(t, resp)
	args := body["args"].(map[string]interface{})
	if args["foo"] != "bar" || args["lang"] != "go" {
		t.Fatalf("unexpected args: %#v", args)
	}
	headers := body["headers"].(map[string]interface{})
	if headers["X-Go-Requests-Test"] != "headers" {
		t.Fatalf("expected custom header, got headers=%#v", headers)
	}
}

func TestHTTPBinIntegrationHTTPMethods(t *testing.T) {
	requireHTTPBinAvailable(t)

	tests := []struct {
		name   string
		method string
		call   func(string) (*requests.Response, error)
	}{
		{name: "POST", method: http.MethodPost, call: func(u string) (*requests.Response, error) {
			return requests.Post(u, requests.JSON{"method": "post"}, requests.Timeout(10*time.Second))
		}},
		{name: "PUT", method: http.MethodPut, call: func(u string) (*requests.Response, error) {
			return requests.Put(u, requests.JSON{"method": "put"}, requests.Timeout(10*time.Second))
		}},
		{name: "PATCH", method: http.MethodPatch, call: func(u string) (*requests.Response, error) {
			return requests.Patch(u, requests.JSON{"method": "patch"}, requests.Timeout(10*time.Second))
		}},
		{name: "DELETE", method: http.MethodDelete, call: func(u string) (*requests.Response, error) {
			return requests.Delete(u, requests.Timeout(10*time.Second))
		}},
		{name: "OPTIONS", method: http.MethodOptions, call: func(u string) (*requests.Response, error) {
			return requests.Options(u, requests.Timeout(10*time.Second))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.call(httpbinURL("/" + strings.ToLower(tt.name)))
			if err != nil {
				t.Fatalf("%s failed: %v", tt.name, err)
			}
			requireStatus(t, resp, http.StatusOK)
		})
	}

	resp, err := requests.Head(httpbinURL("/get"), requests.Timeout(10*time.Second))
	if err != nil {
		t.Fatalf("HEAD /get failed: %v", err)
	}
	requireStatus(t, resp, http.StatusOK)
	if len(resp.Content()) != 0 {
		t.Fatalf("expected empty body for HEAD response, got %q", resp.Text())
	}
}

func TestHTTPBinIntegrationRequestBodies(t *testing.T) {
	requireHTTPBinAvailable(t)

	t.Run("json map", func(t *testing.T) {
		resp, err := requests.Post(httpbinURL("/post"),
			requests.JSON{"name": "alice", "age": 30},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("POST JSON failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		got := decodeHTTPBinJSON(t, resp)["json"].(map[string]interface{})
		if got["name"] != "alice" || got["age"].(float64) != 30 {
			t.Fatalf("unexpected JSON echo: %#v", got)
		}
	})

	t.Run("json struct", func(t *testing.T) {
		type payload struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		}
		resp, err := requests.Post(httpbinURL("/post"),
			requests.JSONBody{Value: payload{Name: "struct-body", OK: true}},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("POST JSONBody failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		got := decodeHTTPBinJSON(t, resp)["json"].(map[string]interface{})
		if got["name"] != "struct-body" || got["ok"] != true {
			t.Fatalf("unexpected JSONBody echo: %#v", got)
		}
	})

	t.Run("form data", func(t *testing.T) {
		resp, err := requests.Post(httpbinURL("/post"),
			requests.Data{"username": "go", "password": "requests"},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("POST form failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		got := decodeHTTPBinJSON(t, resp)["form"].(map[string]interface{})
		if got["username"] != "go" || got["password"] != "requests" {
			t.Fatalf("unexpected form echo: %#v", got)
		}
	})

	t.Run("raw body", func(t *testing.T) {
		resp, err := requests.Post(httpbinURL("/post"),
			requests.Body{Reader: strings.NewReader("raw-body")},
			requests.Headers{"Content-Type": "text/plain"},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("POST raw body failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		got := decodeHTTPBinJSON(t, resp)
		if got["data"] != "raw-body" {
			t.Fatalf("unexpected raw body echo: %#v", got["data"])
		}
	})

	t.Run("multipart files", func(t *testing.T) {
		resp, err := requests.Post(httpbinURL("/post"),
			requests.Data{"description": "integration"},
			requests.Files{"upload": requests.FileField{FileName: "hello.txt", Content: strings.NewReader("hello-httpbin"), ContentType: "text/plain"}},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("POST multipart failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		got := decodeHTTPBinJSON(t, resp)
		files := got["files"].(map[string]interface{})
		form := got["form"].(map[string]interface{})
		if files["upload"] != "hello-httpbin" || form["description"] != "integration" {
			t.Fatalf("unexpected multipart echo: files=%#v form=%#v", files, form)
		}
	})
}

func TestHTTPBinIntegrationAuth(t *testing.T) {
	requireHTTPBinAvailable(t)

	t.Run("basic auth", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/basic-auth/user/pass"),
			requests.Auth{Provider: requests.BasicAuth{Username: "user", Password: "pass"}},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("basic auth request failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		if decodeHTTPBinJSON(t, resp)["authenticated"] != true {
			t.Fatal("expected basic auth to authenticate")
		}
	})

	t.Run("bearer token", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/bearer"),
			requests.Auth{Provider: requests.TokenAuth{Token: "integration-token"}},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("bearer auth request failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		body := decodeHTTPBinJSON(t, resp)
		if body["authenticated"] != true || body["token"] != "integration-token" {
			t.Fatalf("unexpected bearer response: %#v", body)
		}
	})

	t.Run("digest auth", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/digest-auth/auth/user/pass"),
			requests.Auth{Provider: requests.DigestAuth{Username: "user", Password: "pass"}},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("digest auth request failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		if decodeHTTPBinJSON(t, resp)["authenticated"] != true {
			t.Fatal("expected digest auth to authenticate")
		}
	})
}

func TestHTTPBinIntegrationCookiesSessionAndRedirects(t *testing.T) {
	requireHTTPBinAvailable(t)

	t.Run("per request cookies", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/cookies"),
			requests.Cookies{"session": "abc123"},
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("cookies request failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		cookies := decodeHTTPBinJSON(t, resp)["cookies"].(map[string]interface{})
		if cookies["session"] != "abc123" {
			t.Fatalf("expected cookie echo, got %#v", cookies)
		}
	})

	t.Run("session cookie persistence", func(t *testing.T) {
		s := requests.NewSession()
		_, err := s.Get(httpbinURL("/cookies/set?token=session-value"), requests.Timeout(10*time.Second))
		if err != nil {
			t.Fatalf("session cookie set failed: %v", err)
		}
		resp, err := s.Get(httpbinURL("/cookies"), requests.Timeout(10*time.Second))
		if err != nil {
			t.Fatalf("session cookie read failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
		cookies := decodeHTTPBinJSON(t, resp)["cookies"].(map[string]interface{})
		if cookies["token"] != "session-value" {
			t.Fatalf("expected persisted session cookie, got %#v", cookies)
		}
	})

	t.Run("redirect disabled", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/redirect/1"),
			requests.AllowRedirects(false),
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("redirect request failed: %v", err)
		}
		requireStatus(t, resp, http.StatusFound)
		if !resp.IsRedirect() {
			t.Fatal("expected IsRedirect() for 302")
		}
	})

	t.Run("redirect enabled", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/redirect/1"),
			requests.AllowRedirects(true),
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("redirect request failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
	})
}

func TestHTTPBinIntegrationErrorsTimeoutContextAndTLS(t *testing.T) {
	requireHTTPBinAvailable(t)

	t.Run("raise for status", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/status/418"), requests.Timeout(10*time.Second))
		if err != nil {
			t.Fatalf("status request failed: %v", err)
		}
		if resp.Ok() {
			t.Fatal("expected Ok() false for HTTP error")
		}
		if err := resp.RaiseForStatus(); err == nil {
			t.Fatal("expected RaiseForStatus to return an error")
		}
	})

	t.Run("request timeout", func(t *testing.T) {
		_, err := requests.Get(httpbinURL("/delay/3"), requests.Timeout(500*time.Millisecond))
		if err == nil {
			t.Fatal("expected timeout error")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		_, err := requests.Get(httpbinURL("/delay/3"), requests.WithContext{Ctx: ctx})
		if err == nil {
			t.Fatal("expected context deadline error")
		}
	})

	t.Run("tls verify option", func(t *testing.T) {
		resp, err := requests.Get(httpbinURL("/get"),
			requests.Verify(true),
			requests.Timeout(10*time.Second),
		)
		if err != nil {
			t.Fatalf("verified TLS request failed: %v", err)
		}
		requireStatus(t, resp, http.StatusOK)
	})
}
