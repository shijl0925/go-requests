package requests

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Option is implemented by all request-level configuration types.
// Each Option knows how to apply itself to the internal requestConfig.
type Option interface {
	applyOption(*requestConfig)
}

// requestConfig holds the accumulated options for a single request.
type requestConfig struct {
	params         url.Values
	headers        http.Header
	cookies        []*http.Cookie
	auth           AuthProvider
	data           url.Values
	jsonBody       interface{}
	rawBody        io.Reader
	files          map[string]FileField
	timeout        time.Duration
	allowRedirects *bool
	verify         *bool
	proxies        map[string]string
	context        context.Context
}

// FileField represents a file to be uploaded in a multipart request.
type FileField struct {
	// FileName is the file name sent in the Content-Disposition header.
	FileName string
	// Content is the file data reader.
	Content io.Reader
	// ContentType overrides the auto-detected MIME type (optional).
	ContentType string
}

// ---- Option types ---------------------------------------------------------

// Params sets URL query parameters. Multiple Params options are merged.
type Params map[string]string

func (p Params) applyOption(c *requestConfig) {
	if c.params == nil {
		c.params = make(url.Values)
	}
	for k, v := range p {
		c.params.Set(k, v)
	}
}

// Headers sets additional HTTP request headers. Multiple Headers options are
// merged; later values override earlier ones for the same key.
type Headers map[string]string

func (h Headers) applyOption(c *requestConfig) {
	if c.headers == nil {
		c.headers = make(http.Header)
	}
	for k, v := range h {
		c.headers.Set(k, v)
	}
}

// Cookies adds cookies to the request. Multiple Cookies options are merged.
type Cookies map[string]string

func (ck Cookies) applyOption(c *requestConfig) {
	for k, v := range ck {
		c.cookies = append(c.cookies, &http.Cookie{Name: k, Value: v})
	}
}

// Auth sets the authentication provider for the request.
type Auth struct {
	Provider AuthProvider
}

func (a Auth) applyOption(c *requestConfig) {
	c.auth = a.Provider
}

// Data sets the request body as URL-encoded form data and sets
// Content-Type: application/x-www-form-urlencoded.
type Data map[string]string

func (d Data) applyOption(c *requestConfig) {
	if c.data == nil {
		c.data = make(url.Values)
	}
	for k, v := range d {
		c.data.Set(k, v)
	}
}

// JSON sets the request body as JSON-encoded data and sets
// Content-Type: application/json.
type JSON map[string]interface{}

func (j JSON) applyOption(c *requestConfig) {
	c.jsonBody = j
}

// JSONBody sets an arbitrary value (struct, slice, etc.) as the JSON body.
type JSONBody struct {
	Value interface{}
}

func (j JSONBody) applyOption(c *requestConfig) {
	c.jsonBody = j.Value
}

// Body sets a raw io.Reader as the request body. The caller is responsible for
// setting the appropriate Content-Type header.
type Body struct {
	Reader io.Reader
}

func (b Body) applyOption(c *requestConfig) {
	c.rawBody = b.Reader
}

// Files sets multipart file uploads. The map key is the form field name.
type Files map[string]FileField

func (f Files) applyOption(c *requestConfig) {
	if c.files == nil {
		c.files = make(map[string]FileField)
	}
	for k, v := range f {
		c.files[k] = v
	}
}

// Timeout sets the request timeout. It overrides the session-level timeout.
type Timeout time.Duration

func (t Timeout) applyOption(c *requestConfig) {
	c.timeout = time.Duration(t)
}

// AllowRedirects controls whether the client follows HTTP redirects.
type AllowRedirects bool

func (a AllowRedirects) applyOption(c *requestConfig) {
	v := bool(a)
	c.allowRedirects = &v
}

// Verify controls whether TLS certificates are verified.
// Set to false to skip verification (equivalent to Python's verify=False).
type Verify bool

func (v Verify) applyOption(c *requestConfig) {
	b := bool(v)
	c.verify = &b
}

// Proxies sets proxy URLs keyed by scheme (e.g. "http", "https").
type Proxies map[string]string

func (p Proxies) applyOption(c *requestConfig) {
	if c.proxies == nil {
		c.proxies = make(map[string]string)
	}
	for k, v := range p {
		c.proxies[k] = v
	}
}

// WithContext attaches a context to the request. This can be used to cancel or
// set a deadline on the request independently of Timeout.
type WithContext struct {
	Ctx context.Context
}

func (w WithContext) applyOption(c *requestConfig) {
	c.context = w.Ctx
}

// ---- Session ---------------------------------------------------------------

// Session maintains persistent state (headers, cookies, auth, etc.) across
// multiple requests, similar to Python's requests.Session.
type Session struct {
	// Headers are sent with every request made by this session.
	// Per-request Headers options are merged on top of these.
	Headers http.Header

	// Cookies are sent with every request and updated from responses.
	Cookies http.CookieJar

	// Auth is the default authentication provider for the session.
	Auth AuthProvider

	// Timeout is the default timeout for requests. Zero means no timeout.
	Timeout time.Duration

	// AllowRedirects controls whether the client follows redirects.
	// Defaults to true.
	AllowRedirects bool

	// Verify controls TLS certificate verification. Defaults to true.
	Verify bool

	// Proxies maps scheme to proxy URL.
	Proxies map[string]string

	// client is the underlying HTTP client.
	client *http.Client
}

// NewSession creates a new Session with sensible defaults.
func NewSession() *Session {
	jar, _ := newCookieJar()
	s := &Session{
		Headers:        make(http.Header),
		Cookies:        jar,
		AllowRedirects: true,
		Verify:         true,
	}
	s.Headers.Set("User-Agent", "go-requests/1.0")
	s.client = s.buildClient()
	return s
}

// buildClient constructs the http.Client from the current session settings.
func (s *Session) buildClient() *http.Client {
	transport := &http.Transport{}

	if !s.Verify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user-controlled setting
	}

	if len(s.Proxies) > 0 {
		transport.Proxy = func(req *http.Request) (*url.URL, error) {
			scheme := req.URL.Scheme
			if proxyURL, ok := s.Proxies[scheme]; ok {
				return url.Parse(proxyURL)
			}
			return nil, nil
		}
	}

	client := &http.Client{
		Transport: transport,
		Jar:       s.Cookies,
	}

	if !s.AllowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}

// request executes an HTTP request with the given method, URL, and options.
func (s *Session) request(method, rawURL string, opts []Option) (*Response, error) {
	cfg := &requestConfig{}
	for _, o := range opts {
		o.applyOption(cfg)
	}

	// Build URL with query parameters.
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("go-requests: invalid URL %q: %w", rawURL, err)
	}
	if cfg.params != nil {
		q := parsedURL.Query()
		for k, vals := range cfg.params {
			for _, v := range vals {
				q.Set(k, v)
			}
		}
		parsedURL.RawQuery = q.Encode()
	}

	// Build request body.
	body, contentType, err := buildBody(cfg)
	if err != nil {
		return nil, err
	}

	// Create the request.
	ctx := cfg.context
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("go-requests: failed to create request: %w", err)
	}

	// Apply session-level headers first, then per-request headers.
	for k, vals := range s.Headers {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	if cfg.headers != nil {
		for k, vals := range cfg.headers {
			for _, v := range vals {
				req.Header.Set(k, v)
			}
		}
	}

	// Set Content-Type if a body was built.
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Apply cookies.
	for _, ck := range cfg.cookies {
		req.AddCookie(ck)
	}

	// Apply auth (per-request overrides session-level).
	auth := s.Auth
	if cfg.auth != nil {
		auth = cfg.auth
	}
	if auth != nil {
		auth.Apply(req)
	}

	// Determine effective client.
	client := s.buildEffectiveClient(cfg)

	// Apply per-request timeout.
	if cfg.timeout > 0 {
		client.Timeout = cfg.timeout
	} else if s.Timeout > 0 {
		client.Timeout = s.Timeout
	}

	httpResp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("go-requests: request failed: %w", err)
	}

	resp, err := newResponse(httpResp)
	if err != nil {
		return nil, err
	}

	// Handle Digest Auth retry on 401.
	if resp.StatusCode == http.StatusUnauthorized {
		if da, ok := auth.(DigestAuth); ok {
			wwwAuth := httpResp.Header.Get("WWW-Authenticate")
			if strings.HasPrefix(wwwAuth, "Digest ") {
				// Rebuild the request for the retry.
				retryBody, _, err := buildBody(cfg)
				if err != nil {
					return nil, err
				}
				req2, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), retryBody)
				if err != nil {
					return nil, err
				}
				for k, vals := range req.Header {
					req2.Header[k] = vals
				}
				applyDigestAuth(req2, da, wwwAuth)
				httpResp2, err := client.Do(req2)
				if err != nil {
					return nil, fmt.Errorf("go-requests: digest auth retry failed: %w", err)
				}
				resp, err = newResponse(httpResp2)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return resp, nil
}

// buildEffectiveClient returns a client that respects per-request overrides
// (AllowRedirects, Verify, Proxies).
func (s *Session) buildEffectiveClient(cfg *requestConfig) *http.Client {
	// Fast path: no overrides.
	if cfg.allowRedirects == nil && cfg.verify == nil && cfg.proxies == nil {
		return s.client
	}

	// Clone transport.
	base := s.client.Transport.(*http.Transport)
	t := base.Clone()

	if cfg.verify != nil && !*cfg.verify {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	if cfg.proxies != nil {
		proxies := cfg.proxies
		t.Proxy = func(req *http.Request) (*url.URL, error) {
			if u, ok := proxies[req.URL.Scheme]; ok {
				return url.Parse(u)
			}
			return nil, nil
		}
	}

	client := &http.Client{
		Transport: t,
		Jar:       s.client.Jar,
		Timeout:   s.client.Timeout,
	}

	if cfg.allowRedirects != nil && !*cfg.allowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else if s.AllowRedirects {
		client.CheckRedirect = nil
	} else {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}

// buildBody constructs the request body io.Reader and Content-Type from cfg.
// Priority: Files > JSON > Data > RawBody.
func buildBody(cfg *requestConfig) (io.Reader, string, error) {
	if len(cfg.files) > 0 {
		return buildMultipart(cfg)
	}
	if cfg.jsonBody != nil {
		b, err := json.Marshal(cfg.jsonBody)
		if err != nil {
			return nil, "", fmt.Errorf("go-requests: failed to marshal JSON body: %w", err)
		}
		return bytes.NewReader(b), "application/json", nil
	}
	if cfg.data != nil {
		return strings.NewReader(cfg.data.Encode()), "application/x-www-form-urlencoded", nil
	}
	if cfg.rawBody != nil {
		return cfg.rawBody, "", nil
	}
	return http.NoBody, "", nil
}

// buildMultipart creates a multipart/form-data body from files and data fields.
func buildMultipart(cfg *requestConfig) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Write form data fields first.
	if cfg.data != nil {
		for k, vals := range cfg.data {
			for _, v := range vals {
				if err := w.WriteField(k, v); err != nil {
					return nil, "", fmt.Errorf("go-requests: multipart write field %q: %w", k, err)
				}
			}
		}
	}

	// Write file fields.
	for fieldName, ff := range cfg.files {
		filename := ff.FileName
		if filename == "" {
			filename = fieldName
		}
		var fw io.Writer
		var err error
		if ff.ContentType != "" {
			h := make(map[string][]string)
			h["Content-Disposition"] = []string{
				fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filepath.Base(filename)),
			}
			h["Content-Type"] = []string{ff.ContentType}
			fw, err = w.CreatePart(h)
		} else {
			fw, err = w.CreateFormFile(fieldName, filepath.Base(filename))
		}
		if err != nil {
			return nil, "", fmt.Errorf("go-requests: multipart create file %q: %w", fieldName, err)
		}
		if _, err := io.Copy(fw, ff.Content); err != nil {
			return nil, "", fmt.Errorf("go-requests: multipart copy file %q: %w", fieldName, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("go-requests: multipart close: %w", err)
	}
	return &buf, w.FormDataContentType(), nil
}

// ---- HTTP method helpers ---------------------------------------------------

// Get sends a GET request to the specified URL.
func (s *Session) Get(url string, opts ...Option) (*Response, error) {
	return s.request(http.MethodGet, url, opts)
}

// Post sends a POST request to the specified URL.
func (s *Session) Post(url string, opts ...Option) (*Response, error) {
	return s.request(http.MethodPost, url, opts)
}

// Put sends a PUT request to the specified URL.
func (s *Session) Put(url string, opts ...Option) (*Response, error) {
	return s.request(http.MethodPut, url, opts)
}

// Patch sends a PATCH request to the specified URL.
func (s *Session) Patch(url string, opts ...Option) (*Response, error) {
	return s.request(http.MethodPatch, url, opts)
}

// Delete sends a DELETE request to the specified URL.
func (s *Session) Delete(url string, opts ...Option) (*Response, error) {
	return s.request(http.MethodDelete, url, opts)
}

// Head sends a HEAD request to the specified URL.
func (s *Session) Head(url string, opts ...Option) (*Response, error) {
	return s.request(http.MethodHead, url, opts)
}

// Options sends an OPTIONS request to the specified URL.
func (s *Session) Options(url string, opts ...Option) (*Response, error) {
	return s.request(http.MethodOptions, url, opts)
}

// Request sends an HTTP request with an arbitrary method.
func (s *Session) Request(method, url string, opts ...Option) (*Response, error) {
	return s.request(method, url, opts)
}
