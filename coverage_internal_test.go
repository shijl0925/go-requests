package requests

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const incompleteIPv6URL = "http://[::1"

func TestTopLevelAndSessionRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Method)
	}))
	defer srv.Close()

	resp, err := Request(http.MethodTrace, srv.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.Text() != http.MethodTrace {
		t.Fatalf("expected TRACE, got %q", resp.Text())
	}

	resp, err = NewSession().Request(http.MethodConnect, srv.URL)
	if err != nil {
		t.Fatalf("Session.Request failed: %v", err)
	}
	if resp.Text() != http.MethodConnect {
		t.Fatalf("expected CONNECT, got %q", resp.Text())
	}
}

func TestOptionApplyBranches(t *testing.T) {
	cfg := &requestConfig{}
	Verify(false).applyOption(cfg)
	if cfg.verify == nil || *cfg.verify {
		t.Fatal("Verify(false) did not set request config")
	}

	Proxies{"http": "http://proxy.test:8080"}.applyOption(cfg)
	if cfg.proxies["http"] != "http://proxy.test:8080" {
		t.Fatalf("proxy not applied: %#v", cfg.proxies)
	}

	tc := TransportConfig{MaxIdleConns: 1}
	tc.applyOption(cfg)
	if cfg.transport == nil || cfg.transport.MaxIdleConns != 1 {
		t.Fatalf("transport config not applied: %#v", cfg.transport)
	}

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, errors.New("unused") })
	RoundTripper{Transport: rt}.applyOption(cfg)
	if cfg.roundTripper == nil {
		t.Fatal("round tripper not applied")
	}
}

func TestSessionSetCookieInvalidURLAndResetClient(t *testing.T) {
	s := NewSession()
	if err := s.SetCookie(incompleteIPv6URL, "testCookie", "value"); err == nil || !strings.Contains(err.Error(), "invalid cookie URL") {
		t.Fatal("expected invalid URL error")
	}

	custom := &http.Client{Timeout: time.Second}
	s.SetClient(custom)
	if !s.customClient || s.client != custom {
		t.Fatal("expected custom client to be installed")
	}
	s.SetClient(nil)
	if s.customClient || s.client == custom || s.client == nil {
		t.Fatal("expected nil client to restore default client")
	}
}

func TestBuildClientAndTransportBranches(t *testing.T) {
	s := NewSession()
	s.AllowRedirects = false
	client := s.buildClientLocked()
	if client.CheckRedirect == nil {
		t.Fatal("expected redirect policy for disabled redirects")
	}

	customRT := roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, errors.New("unused") })
	s.RoundTripper = customRT
	if got := s.buildTransportLocked(); got == nil {
		t.Fatal("expected custom non-transport round tripper, got nil")
	} else if _, ok := got.(roundTripFunc); !ok {
		t.Fatalf("expected custom non-transport round tripper, got %T", got)
	}

	s.RoundTripper = &http.Transport{TLSClientConfig: &tls.Config{ServerName: "example.test"}}
	s.Verify = false
	s.Proxies = map[string]string{"https": "http://proxy.test:8080"}
	s.TransportConfig = &TransportConfig{
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   4,
		MaxConnsPerHost:       3,
		IdleConnTimeout:       time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		ExpectContinueTimeout: 4 * time.Second,
	}
	tr, ok := s.buildTransportLocked().(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport")
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected insecure TLS config")
	}
	if tr.MaxIdleConns != 10 || tr.MaxIdleConnsPerHost != 4 || tr.MaxConnsPerHost != 3 ||
		tr.IdleConnTimeout != time.Second || tr.TLSHandshakeTimeout != 2*time.Second ||
		tr.ResponseHeaderTimeout != 3*time.Second || tr.ExpectContinueTimeout != 4*time.Second {
		t.Fatalf("transport config not applied: %#v", tr)
	}
	proxy, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https"}})
	if err != nil || proxy.String() != "http://proxy.test:8080" {
		t.Fatalf("unexpected proxy: %v %v", proxy, err)
	}
	proxy, err = tr.Proxy(&http.Request{URL: &url.URL{Scheme: "http"}})
	if err != nil || proxy != nil {
		t.Fatalf("expected no proxy for http, got %v %v", proxy, err)
	}
}

func TestBuildEffectiveClientOverrides(t *testing.T) {
	jar, _ := newCookieJar()
	baseTransport := &http.Transport{}
	snap := sessionSnapshot{
		cookies:        jar,
		client:         &http.Client{Transport: baseTransport},
		allowRedirects: true,
	}

	verify := false
	cfg := &requestConfig{
		verify:    &verify,
		proxies:   map[string]string{"http": "http://proxy.local:3128"},
		transport: &TransportConfig{ResponseHeaderTimeout: time.Second},
	}
	client := snap.buildEffectiveClient(cfg)
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected transport clone, got %T", client.Transport)
	}
	if tr == baseTransport || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify ||
		tr.ResponseHeaderTimeout != time.Second || client.Jar != jar {
		t.Fatalf("unexpected effective client: %#v", client)
	}

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) { return nil, errors.New("unused") })
	client = snap.buildEffectiveClient(&requestConfig{client: &http.Client{}, roundTripper: rt})
	if client.Transport == nil || client.Jar != jar {
		t.Fatalf("expected round tripper override and cookie jar: %#v", client)
	}
}

func TestSessionTransportSettersAndSnapshot(t *testing.T) {
	s := NewSession().
		SetVerify(false).
		SetProxies(map[string]string{"https": "http://proxy.test:8080"}).
		SetTransportConfig(TransportConfig{ResponseHeaderTimeout: time.Second})

	snap := s.snapshot()
	client := snap.buildEffectiveClient(&requestConfig{})
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected transport, got %T", client.Transport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected SetVerify(false) to affect effective transport")
	}
	if tr.ResponseHeaderTimeout != time.Second {
		t.Fatalf("expected transport config timeout, got %s", tr.ResponseHeaderTimeout)
	}
	proxy, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https"}})
	if err != nil || proxy.String() != "http://proxy.test:8080" {
		t.Fatalf("unexpected proxy: %v %v", proxy, err)
	}

	s.Proxies["https"] = "http://changed.test:8080"
	proxy, err = tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https"}})
	if err != nil || proxy.String() != "http://proxy.test:8080" {
		t.Fatalf("snapshot should isolate proxy map, got %v %v", proxy, err)
	}
}

func TestCloneHelpers(t *testing.T) {
	if got := cloneStringMap(nil); got != nil {
		t.Fatalf("expected nil map clone, got %#v", got)
	}
	if got := cloneStringMap(map[string]string{}); got != nil {
		t.Fatalf("expected empty map clone to be nil, got %#v", got)
	}
	originalMap := map[string]string{"https": "http://proxy.test:8080"}
	clonedMap := cloneStringMap(originalMap)
	originalMap["https"] = "http://changed.test:8080"
	if clonedMap["https"] != "http://proxy.test:8080" {
		t.Fatalf("clone should not share map storage: %#v", clonedMap)
	}

	if got := cloneTransportConfig(nil); got != nil {
		t.Fatalf("expected nil transport config clone, got %#v", got)
	}
	originalConfig := &TransportConfig{ResponseHeaderTimeout: time.Second}
	clonedConfig := cloneTransportConfig(originalConfig)
	originalConfig.ResponseHeaderTimeout = 2 * time.Second
	if clonedConfig == originalConfig || clonedConfig.ResponseHeaderTimeout != time.Second {
		t.Fatalf("unexpected cloned config: %#v", clonedConfig)
	}
}

func TestNewHTTPRequestErrorsAndHeaderPrecedence(t *testing.T) {
	snap := sessionSnapshot{headers: http.Header{"Content-Type": {"from-session"}, "X-Session": {"yes"}}}
	cfg := &requestConfig{
		jsonBody:    map[string]string{"hello": "world"},
		headers:     http.Header{"Content-Type": {"from-request"}, "X-Request": {"yes"}},
		contentType: "from-content-type-option",
		cookies:     []*http.Cookie{{Name: "a", Value: "b"}},
	}
	req, err := snap.newHTTPRequest(context.Background(), http.MethodPost, "http://example.test", cfg, BasicAuth{Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("newHTTPRequest failed: %v", err)
	}
	if req.Header.Get("Content-Type") != "from-content-type-option" ||
		req.Header.Get("X-Session") != "yes" || req.Header.Get("X-Request") != "yes" ||
		req.Header.Get("Authorization") == "" {
		t.Fatalf("unexpected headers: %#v", req.Header)
	}
	if _, err := req.Cookie("a"); err != nil {
		t.Fatalf("cookie missing: %v", err)
	}

	if _, err := snap.newHTTPRequest(context.Background(), http.MethodGet, incompleteIPv6URL, &requestConfig{}, nil); err == nil {
		t.Fatal("expected invalid request URL error")
	}
}

func TestRequestErrorPathsAndRetries(t *testing.T) {
	t.Run("invalid URL", func(t *testing.T) {
		if _, err := NewSession().Get(incompleteIPv6URL); err == nil {
			t.Fatal("expected invalid URL error")
		}
	})

	t.Run("new response read error", func(t *testing.T) {
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       errReadCloser{err: errors.New("read failed")},
				Request:    req,
			}, nil
		}))
		if _, err := s.Get("http://example.test"); err == nil {
			t.Fatal("expected read error")
		}
	})

	t.Run("transport error retried then succeeds", func(t *testing.T) {
		attempts := 0
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("temporary")
			}
			return okResponse(req, "ok"), nil
		}))
		resp, err := s.Get("http://example.test", Retry{MaxRetries: 1})
		if err != nil {
			t.Fatalf("expected retry success: %v", err)
		}
		if attempts != 2 || resp.Text() != "ok" {
			t.Fatalf("unexpected retry result attempts=%d body=%q", attempts, resp.Text())
		}
	})

	t.Run("context canceled during retry wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("temporary")
		}))
		_, err := s.Get("http://example.test", WithContext(ctx), Retry{MaxRetries: 1, Wait: time.Hour})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	})

	t.Run("non replayable body is not retried", func(t *testing.T) {
		attempts := 0
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			return nil, errors.New("temporary")
		}))
		if _, err := s.Post("http://example.test", Body{Reader: strings.NewReader("body")}, Retry{MaxRetries: 3}); err == nil {
			t.Fatal("expected request error")
		}
		if attempts != 1 {
			t.Fatalf("expected no retry, got %d attempts", attempts)
		}
	})
}

func TestDigestAuthAdditionalBranches(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://example.test/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	DigestAuth{Username: "u", Password: "p"}.Apply(req)
	if req.Header.Get("Authorization") != "" {
		t.Fatal("DigestAuth.Apply should be a no-op")
	}
	for _, tc := range []struct {
		name      string
		challenge string
		want      string
	}{
		{name: "qop", challenge: `Digest realm="r", nonce="n", qop="auth-int"`, want: "unsupported digest auth qop"},
		{name: "algorithm", challenge: `Digest realm="r", nonce="n", algorithm=SHA-256`, want: "unsupported digest auth algorithm"},
	} {
		t.Run("unsupported "+tc.name, func(t *testing.T) {
			if err := applyDigestAuth(req, DigestAuth{Username: "u", Password: "p"}, tc.challenge); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
	if got := selectDigestQOP("auth-int, auth"); got != "auth" {
		t.Fatalf("expected auth qop, got %q", got)
	}
	if got := selectDigestQOP("auth"); got != "auth" {
		t.Fatalf("expected single auth qop, got %q", got)
	}
	if got := parseDigestChallenge(`Digest badpart, realm="r"`); got["realm"] != "r" || got["badpart"] != "" {
		t.Fatalf("unexpected parsed challenge: %#v", got)
	}
	parts := splitDigestChallenge(`realm="a\",b", nonce="n"`)
	if len(parts) != 2 {
		t.Fatalf("expected escaped comma to stay quoted, got %#v", parts)
	}
}

func TestNewHTTPRequestPreservesRequestHeaderValues(t *testing.T) {
	s := sessionSnapshot{
		headers: http.Header{"Accept": {"session"}},
	}
	req, err := s.newHTTPRequest(context.Background(), http.MethodGet, "http://example.test", &requestConfig{
		headers: http.Header{"Accept": {"text/plain", "application/json"}},
	}, nil)
	if err != nil {
		t.Fatalf("newHTTPRequest failed: %v", err)
	}
	got := req.Header.Values("Accept")
	want := []string{"text/plain", "application/json"}
	if len(got) != len(want) {
		t.Fatalf("unexpected Accept values: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected Accept values: got %#v want %#v", got, want)
		}
	}
}

func TestRetryHelpers(t *testing.T) {
	if retryMaxAttempts(nil) != 1 || retryMaxAttempts(&Retry{MaxRetries: -1}) != 1 || retryMaxAttempts(&Retry{MaxRetries: 2}) != 3 {
		t.Fatal("unexpected max attempts")
	}
	if !shouldRetryResponse(&Response{StatusCode: http.StatusTooManyRequests}, &requestConfig{retry: &Retry{MaxRetries: 1}}, http.MethodGet, 0, 2) {
		t.Fatal("expected default status retry")
	}
	if shouldRetryResponse(&Response{StatusCode: http.StatusBadRequest}, &requestConfig{retry: &Retry{MaxRetries: 1}}, http.MethodGet, 0, 2) {
		t.Fatal("did not expect retry for 400")
	}
	cfg := &requestConfig{retry: &Retry{MaxRetries: 1, Methods: []string{http.MethodPost}}}
	if retryAllowed(cfg, http.MethodGet) || !retryAllowed(cfg, http.MethodPost) {
		t.Fatal("method retry filter failed")
	}
	fileCfg := &requestConfig{
		retry: &Retry{MaxRetries: 1, Methods: []string{http.MethodPost}},
		files: map[string]FileField{"f": {Content: strings.NewReader("x")}},
	}
	if retryAllowed(fileCfg, http.MethodPost) {
		t.Fatal("file uploads should not be retryable")
	}
	if got := retryStatusCodes(&Retry{StatusCodes: []int{418}}); len(got) != 1 || got[0] != 418 {
		t.Fatalf("unexpected status codes: %#v", got)
	}
	if retryDelay(nil, 0) != 0 || retryDelay(&Retry{Wait: 0}, 0) != 0 {
		t.Fatal("expected zero retry delay")
	}
	if got := retryDelay(&Retry{Wait: time.Second, BackoffFactor: 2, MaxWait: 1500 * time.Millisecond}, 2); got != 1500*time.Millisecond {
		t.Fatalf("expected capped delay, got %s", got)
	}
	if got := retryDelay(&Retry{Wait: time.Second}, 1); got != time.Second {
		t.Fatalf("expected default factor delay, got %s", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(waitBeforeRetry(ctx, &Retry{Wait: time.Hour}, 0), context.Canceled) {
		t.Fatal("expected canceled wait")
	}
}

func TestResponseEdgeCases(t *testing.T) {
	t.Run("nil raw response", func(t *testing.T) {
		resp := &Response{}
		if resp.Body() != nil {
			t.Fatal("expected nil body")
		}
		if err := resp.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
		content, err := resp.ReadContent()
		if err != nil || content != nil || !resp.bodyRead {
			t.Fatalf("unexpected ReadContent: %q %v", content, err)
		}
		path := t.TempDir() + "/empty"
		if err := resp.SaveToFile(path); err != nil {
			t.Fatalf("SaveToFile nil body failed: %v", err)
		}
		if data, _ := os.ReadFile(path); len(data) != 0 {
			t.Fatalf("expected empty file, got %q", data)
		}
	})

	t.Run("stream read and save errors", func(t *testing.T) {
		resp := &Response{RawResponse: &http.Response{Body: errReadCloser{err: errors.New("boom")}}}
		if _, err := resp.ReadContent(); err == nil {
			t.Fatal("expected ReadContent error")
		}
		if got := resp.Content(); got != nil {
			t.Fatalf("expected nil content after read error, got %q", got)
		}

		resp = &Response{RawResponse: &http.Response{Body: errReadCloser{err: errors.New("copy boom")}}}
		if err := resp.SaveToFile(t.TempDir() + "/out"); err == nil {
			t.Fatal("expected SaveToFile copy error")
		}
		resp = &Response{RawResponse: &http.Response{Body: closeErrReadCloser{Reader: strings.NewReader("ok")}}}
		if err := resp.SaveToFile(t.TempDir() + "/out"); err == nil {
			t.Fatal("expected SaveToFile close error")
		}
		if err := (&Response{bodyRead: true, body: []byte("x")}).SaveToFile(t.TempDir()); err == nil {
			t.Fatal("expected write file error for directory")
		}
	})

	t.Run("encoding and errors", func(t *testing.T) {
		resp := &Response{
			StatusCode: http.StatusTeapot,
			Status:     "418 I'm a teapot",
			URL:        mustParseURL(t, "http://example.test/tea"),
			Headers:    http.Header{"Content-Type": {"text/plain; charset=ISO-8859-1"}},
			body:       []byte("{bad"),
			bodyRead:   true,
		}
		if got := resp.Encoding(); got != "iso-8859-1" {
			t.Fatalf("unexpected encoding %q", got)
		}
		var out map[string]any
		if err := resp.JSON(&out); err == nil {
			t.Fatal("expected JSON error")
		}
		err := resp.RaiseForStatus()
		if err == nil || err.Error() != "418 I'm a teapot: http://example.test/tea" {
			t.Fatalf("unexpected HTTPError: %v", err)
		}
		if (&Response{StatusCode: http.StatusOK}).RaiseForStatus() != nil {
			t.Fatal("did not expect status error")
		}
		if (&Response{Headers: http.Header{}}).Encoding() != "utf-8" {
			t.Fatal("expected default encoding")
		}
	})
}

func TestBuildBodyAndMultipartBranches(t *testing.T) {
	if body, contentType, err := buildBody(&requestConfig{}); err != nil || body != http.NoBody || contentType != "" {
		t.Fatalf("unexpected empty body: %T %q %v", body, contentType, err)
	}
	if _, _, err := buildBody(&requestConfig{jsonBody: make(chan int)}); err == nil || !strings.Contains(err.Error(), "failed to marshal JSON body") {
		t.Fatalf("expected JSON marshal error, got %v", err)
	}
	if body, contentType, err := buildBody(&requestConfig{data: url.Values{"a": {"b"}}}); err != nil || contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected form body: %q %v", contentType, err)
	} else if data, _ := io.ReadAll(body); string(data) != "a=b" {
		t.Fatalf("unexpected form data %q", data)
	}
	if body, contentType, err := buildBody(&requestConfig{rawBody: strings.NewReader("raw")}); err != nil || contentType != "" {
		t.Fatalf("unexpected raw body: %q %v", contentType, err)
	} else if data, _ := io.ReadAll(body); string(data) != "raw" {
		t.Fatalf("unexpected raw data %q", data)
	}

	cfg := &requestConfig{
		data: url.Values{"field": {"value"}},
		files: map[string]FileField{
			"file": {FileName: "file", Content: strings.NewReader("data"), ContentType: "text/custom"},
		},
	}
	body, contentType, err := buildMultipart(cfg)
	if err != nil {
		t.Fatalf("buildMultipart failed: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("unexpected multipart content type %q", contentType)
	}
	mr := multipart.NewReader(body, strings.TrimPrefix(contentType, "multipart/form-data; boundary="))
	form, err := mr.ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("ReadForm failed: %v", err)
	}
	if form.Value["field"][0] != "value" {
		t.Fatalf("unexpected multipart field value: %#v", form.Value)
	}
	if form.File["file"][0].Filename != "file" {
		t.Fatalf("unexpected multipart filename: %#v", form.File["file"][0])
	}
	if form.File["file"][0].Header.Get("Content-Type") != "text/custom" {
		t.Fatalf("unexpected multipart content type: %#v", form.File["file"][0].Header)
	}

	pr, pw := io.Pipe()
	_ = pr.Close()
	w := multipart.NewWriter(pw)
	if err := writeMultipart(w, &requestConfig{data: url.Values{"field": {"value"}}}); err == nil {
		t.Fatal("expected write field error")
	}
	_ = w.Close()
	_ = pw.Close()

	w = multipart.NewWriter(io.Discard)
	if err := writeMultipart(w, &requestConfig{files: map[string]FileField{"file": {Content: errReader{}}}}); err == nil {
		t.Fatal("expected file copy error")
	}
	if err := writeMultipart(w, &requestConfig{files: map[string]FileField{"file": {}}}); err == nil || !strings.Contains(err.Error(), "nil content") {
		t.Fatalf("expected nil content error, got %v", err)
	}
}

func TestMultipartCustomContentTypeEscapesDisposition(t *testing.T) {
	fieldName := `fi"le\name`
	fileName := `a"b\c.txt`
	cfg := &requestConfig{
		files: map[string]FileField{
			fieldName: {
				FileName:    fileName,
				Content:     strings.NewReader("file-content"),
				ContentType: "text/plain",
			},
		},
	}
	body, contentType, err := buildBody(cfg)
	if err != nil {
		t.Fatalf("buildBody failed: %v", err)
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read multipart body failed: %v", err)
	}
	boundary := strings.TrimPrefix(contentType, "multipart/form-data; boundary=")
	form, err := multipart.NewReader(bytes.NewReader(raw), boundary).ReadForm(1024)
	if err != nil {
		t.Fatalf("parse multipart failed: %v\n%s", err, string(raw))
	}
	defer form.RemoveAll()
	files := form.File[fieldName]
	if len(files) != 1 {
		t.Fatalf("expected one file for %q, got %#v", fieldName, form.File)
	}
	if files[0].Filename != fileName {
		t.Fatalf("unexpected filename: %q", files[0].Filename)
	}
	if got := files[0].Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("unexpected content type: %q", got)
	}
}

func TestDigestRetryErrors(t *testing.T) {
	t.Run("close challenge response error", func(t *testing.T) {
		calls := 0
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			header := make(http.Header)
			header.Set("WWW-Authenticate", `Digest realm="test", nonce="nonce"`)
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     header,
				Body:       closeErrReadCloser{Reader: strings.NewReader("unauthorized")},
				Request:    req,
			}, nil
		}))
		_, err := s.Get("http://example.test", Stream(true), Auth{Provider: DigestAuth{Username: "u", Password: "p"}})
		if err == nil || !strings.Contains(err.Error(), "close digest challenge response") || calls != 1 {
			t.Fatalf("unexpected error/calls: %v %d", err, calls)
		}
	})

	t.Run("retry request fails", func(t *testing.T) {
		calls := 0
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				header := make(http.Header)
				header.Set("WWW-Authenticate", `Digest realm="test", nonce="nonce"`)
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Status:     "401 Unauthorized",
					Header:     header,
					Body:       io.NopCloser(strings.NewReader("unauthorized")),
					Request:    req,
				}, nil
			}
			return nil, errors.New("retry failed")
		}))
		_, err := s.Get("http://example.test", Auth{Provider: DigestAuth{Username: "u", Password: "p"}})
		if err == nil || !strings.Contains(err.Error(), "digest auth retry failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("retry response read fails", func(t *testing.T) {
		calls := 0
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				header := make(http.Header)
				header.Set("WWW-Authenticate", `Digest realm="test", nonce="nonce"`)
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Status:     "401 Unauthorized",
					Header:     header,
					Body:       io.NopCloser(strings.NewReader("unauthorized")),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       errReadCloser{err: errors.New("retry body failed")},
				Request:    req,
			}, nil
		}))
		if _, err := s.Get("http://example.test", Auth{Provider: DigestAuth{Username: "u", Password: "p"}}); err == nil {
			t.Fatal("expected retry read error")
		}
	})

	t.Run("non replayable body rejected", func(t *testing.T) {
		calls := 0
		s := NewSession().SetRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			header := make(http.Header)
			header.Set("WWW-Authenticate", `Digest realm="test", nonce="nonce"`)
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     header,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Request:    req,
			}, nil
		}))
		_, err := s.Post("http://example.test", Body{Reader: strings.NewReader("body")}, Auth{Provider: DigestAuth{Username: "u", Password: "p"}})
		if err == nil || !strings.Contains(err.Error(), "digest auth requires replayable request body") {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Fatalf("expected one request, got %d", calls)
		}
	})
}

func okResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

type errReadCloser struct {
	err error
}

func (e errReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e errReadCloser) Close() error {
	return nil
}

type closeErrReadCloser struct {
	io.Reader
}

func (c closeErrReadCloser) Close() error {
	return errors.New("close failed")
}
