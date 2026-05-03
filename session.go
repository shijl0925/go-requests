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
	maxRedirects   *int
	verify         *bool
	proxies        map[string]string
	context        context.Context
	stream         bool
	contentType    string
	retry          *Retry
	transport      *TransportConfig
	client         *http.Client
	roundTripper   http.RoundTripper
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

// ParamValues sets URL query parameters with support for repeated keys.
type ParamValues map[string][]string

func (p ParamValues) applyOption(c *requestConfig) {
	if c.params == nil {
		c.params = make(url.Values)
	}
	for k, vals := range p {
		for _, v := range vals {
			c.params.Add(k, v)
		}
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

// Header sets one HTTP request header.
type Header struct {
	Name  string
	Value string
}

func (h Header) applyOption(c *requestConfig) {
	Headers{h.Name: h.Value}.applyOption(c)
}

// UserAgent sets the User-Agent request header.
type UserAgent string

func (u UserAgent) applyOption(c *requestConfig) {
	Header{Name: "User-Agent", Value: string(u)}.applyOption(c)
}

// Referer sets the Referer request header.
type Referer string

func (r Referer) applyOption(c *requestConfig) {
	Header{Name: "Referer", Value: string(r)}.applyOption(c)
}

// Accept sets the Accept request header.
type Accept string

func (a Accept) applyOption(c *requestConfig) {
	Header{Name: "Accept", Value: string(a)}.applyOption(c)
}

// ContentType overrides the Content-Type request header.
type ContentType string

func (ct ContentType) applyOption(c *requestConfig) {
	c.contentType = string(ct)
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

// DataValues sets URL-encoded form data with support for repeated keys.
type DataValues map[string][]string

func (d DataValues) applyOption(c *requestConfig) {
	if c.data == nil {
		c.data = make(url.Values)
	}
	for k, vals := range d {
		for _, v := range vals {
			c.data.Add(k, v)
		}
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

// Stream controls whether the response body is left open for streaming.
// Set to true for large downloads, similar to Python requests' stream=True.
type Stream bool

func (s Stream) applyOption(c *requestConfig) {
	c.stream = bool(s)
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

// MaxRedirects limits the number of redirects followed for a request.
// Set to 0 to return the first redirect response without following it.
type MaxRedirects int

func (m MaxRedirects) applyOption(c *requestConfig) {
	v := int(m)
	c.maxRedirects = &v
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

// TransportConfig exposes common http.Transport connection-pool and timeout
// settings. Zero-value fields leave the underlying transport defaults intact.
type TransportConfig struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	ExpectContinueTimeout time.Duration
}

func (tc TransportConfig) applyOption(c *requestConfig) {
	c.transport = &tc
}

// HTTPClient uses a custom *http.Client for a single request.
type HTTPClient struct {
	Client *http.Client
}

func (hc HTTPClient) applyOption(c *requestConfig) {
	c.client = hc.Client
}

// RoundTripper uses a custom http.RoundTripper for a single request.
type RoundTripper struct {
	Transport http.RoundTripper
}

func (rt RoundTripper) applyOption(c *requestConfig) {
	c.roundTripper = rt.Transport
}

// WithContext attaches a context to the request. This can be used to cancel or
// set a deadline on the request independently of Timeout.
type WithContext struct {
	Ctx context.Context
}

func (w WithContext) applyOption(c *requestConfig) {
	c.context = w.Ctx
}

// Retry configures automatic retries for replayable requests.
type Retry struct {
	// MaxRetries is the number of retry attempts after the initial request.
	MaxRetries int
	// Wait is the initial delay between attempts.
	Wait time.Duration
	// MaxWait caps the computed delay when BackoffFactor is used.
	MaxWait time.Duration
	// BackoffFactor multiplies the delay after each retry. Values <= 0 use 1.
	BackoffFactor float64
	// StatusCodes lists response status codes that should be retried. When
	// empty, 429, 500, 502, 503, and 504 are retried.
	StatusCodes []int
	// Methods lists HTTP methods that may be retried. When empty, all methods
	// are eligible if the request body can be replayed.
	Methods []string
}

func (r Retry) applyOption(c *requestConfig) {
	c.retry = &r
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

	// TransportConfig configures the default HTTP transport.
	TransportConfig *TransportConfig

	// RoundTripper is the default transport used by this session. When set to a
	// non-*http.Transport value, Verify, Proxies, and TransportConfig are not
	// applied to it.
	RoundTripper http.RoundTripper

	// client is the underlying HTTP client.
	client *http.Client
	// customClient indicates client was provided by the caller.
	customClient bool
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

// SetHeader sets a header sent with every request made by this session.
func (s *Session) SetHeader(name, value string) *Session {
	s.Headers.Set(name, value)
	return s
}

// SetCookie stores a cookie for the given URL in this session's cookie jar.
func (s *Session) SetCookie(rawURL, name, value string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("go-requests: invalid cookie URL %q: %w", rawURL, err)
	}
	s.Cookies.SetCookies(u, []*http.Cookie{{Name: name, Value: value}})
	return nil
}

// SetAuth sets the default authentication provider for this session.
func (s *Session) SetAuth(auth AuthProvider) *Session {
	s.Auth = auth
	return s
}

// SetTimeout sets the default timeout for this session.
func (s *Session) SetTimeout(timeout time.Duration) *Session {
	s.Timeout = timeout
	return s
}

// SetTransportConfig configures the default HTTP transport for this session.
func (s *Session) SetTransportConfig(config TransportConfig) *Session {
	s.TransportConfig = &config
	if !s.customClient {
		s.client = s.buildClient()
	}
	return s
}

// SetClient sets the underlying HTTP client for this session.
// Passing nil restores the default client built from session settings.
func (s *Session) SetClient(client *http.Client) *Session {
	if client == nil {
		s.customClient = false
		s.client = s.buildClient()
		return s
	}
	s.client = client
	s.customClient = true
	return s
}

// SetRoundTripper sets the default RoundTripper for this session.
func (s *Session) SetRoundTripper(roundTripper http.RoundTripper) *Session {
	s.RoundTripper = roundTripper
	if !s.customClient {
		s.client = s.buildClient()
	}
	return s
}

// buildClient constructs the http.Client from the current session settings.
func (s *Session) buildClient() *http.Client {
	client := &http.Client{
		Transport: s.buildTransport(),
		Jar:       s.Cookies,
	}

	if !s.AllowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}

func (s *Session) buildTransport() http.RoundTripper {
	transport, ok := cloneHTTPTransport(s.RoundTripper)
	if !ok {
		return s.RoundTripper
	}

	if !s.Verify {
		setInsecureSkipVerify(transport, true)
	}

	applyProxies(transport, s.Proxies)
	applyTransportConfig(transport, s.TransportConfig)
	return transport
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
			q.Del(k)
			for _, v := range vals {
				q.Add(k, v)
			}
		}
		parsedURL.RawQuery = q.Encode()
	}

	ctx := cfg.context
	if ctx == nil {
		ctx = context.Background()
	}

	// Determine effective client.
	client := s.buildEffectiveClient(cfg)

	// Apply per-request timeout.
	if cfg.timeout > 0 {
		client.Timeout = cfg.timeout
	} else if s.Timeout > 0 {
		client.Timeout = s.Timeout
	}

	auth := s.Auth
	if cfg.auth != nil {
		auth = cfg.auth
	}
	start := time.Now()
	maxAttempts := retryMaxAttempts(cfg.retry)
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := s.newHTTPRequest(ctx, method, parsedURL.String(), cfg, auth)
		if err != nil {
			return nil, err
		}

		httpResp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if shouldRetryError(cfg, method, attempt, maxAttempts) {
				if err := waitBeforeRetry(ctx, cfg.retry, attempt); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("go-requests: request failed: %w", err)
		}

		resp, err := newResponse(httpResp, cfg.stream)
		if err != nil {
			return nil, err
		}
		resp.Elapsed = time.Since(start)

		// Handle Digest Auth retry on 401.
		if resp.StatusCode == http.StatusUnauthorized {
			if da, ok := auth.(DigestAuth); ok {
				wwwAuth := httpResp.Header.Get("WWW-Authenticate")
				if strings.HasPrefix(wwwAuth, "Digest ") {
					_ = resp.Close()
					resp, err = s.retryDigestAuth(ctx, client, req, method, parsedURL.String(), cfg, da, wwwAuth, start)
					if err != nil {
						return nil, err
					}
				}
			}
		}

		if shouldRetryResponse(resp, cfg, method, attempt, maxAttempts) {
			_ = resp.Close()
			if err := waitBeforeRetry(ctx, cfg.retry, attempt); err != nil {
				return nil, err
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("go-requests: request failed: %w", lastErr)
}

func (s *Session) newHTTPRequest(ctx context.Context, method, rawURL string, cfg *requestConfig, auth AuthProvider) (*http.Request, error) {
	body, contentType, err := buildBody(cfg)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		if pr, ok := body.(*io.PipeReader); ok {
			_ = pr.CloseWithError(err)
		}
		return nil, fmt.Errorf("go-requests: failed to create request: %w", err)
	}

	for k, vals := range s.Headers {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cfg.headers != nil {
		for k, vals := range cfg.headers {
			for _, v := range vals {
				req.Header.Set(k, v)
			}
		}
	}
	if cfg.contentType != "" {
		req.Header.Set("Content-Type", cfg.contentType)
	}

	for _, ck := range cfg.cookies {
		req.AddCookie(ck)
	}
	if auth != nil {
		auth.Apply(req)
	}

	return req, nil
}

func (s *Session) retryDigestAuth(ctx context.Context, client *http.Client, req *http.Request, method, rawURL string, cfg *requestConfig, da DigestAuth, wwwAuth string, start time.Time) (*Response, error) {
	retryReq, err := s.newHTTPRequest(ctx, method, rawURL, cfg, nil)
	if err != nil {
		return nil, err
	}
	for k, vals := range req.Header {
		retryReq.Header[k] = vals
	}
	applyDigestAuth(retryReq, da, wwwAuth)
	httpResp, err := client.Do(retryReq)
	if err != nil {
		return nil, fmt.Errorf("go-requests: digest auth retry failed: %w", err)
	}
	resp, err := newResponse(httpResp, cfg.stream)
	if err != nil {
		return nil, err
	}
	resp.Elapsed = time.Since(start)
	return resp, nil
}

func retryMaxAttempts(retry *Retry) int {
	if retry == nil || retry.MaxRetries <= 0 {
		return 1
	}
	return retry.MaxRetries + 1
}

func shouldRetryError(cfg *requestConfig, method string, attempt, maxAttempts int) bool {
	return attempt+1 < maxAttempts && retryAllowed(cfg, method)
}

func shouldRetryResponse(resp *Response, cfg *requestConfig, method string, attempt, maxAttempts int) bool {
	if attempt+1 >= maxAttempts || !retryAllowed(cfg, method) {
		return false
	}
	for _, code := range retryStatusCodes(cfg.retry) {
		if resp.StatusCode == code {
			return true
		}
	}
	return false
}

func retryAllowed(cfg *requestConfig, method string) bool {
	if cfg.retry == nil || !isReplayable(cfg) {
		return false
	}
	if len(cfg.retry.Methods) == 0 {
		return true
	}
	for _, m := range cfg.retry.Methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

func isReplayable(cfg *requestConfig) bool {
	return cfg.rawBody == nil && len(cfg.files) == 0
}

func retryStatusCodes(retry *Retry) []int {
	if retry != nil && len(retry.StatusCodes) > 0 {
		return retry.StatusCodes
	}
	return []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
}

func waitBeforeRetry(ctx context.Context, retry *Retry, attempt int) error {
	delay := retryDelay(retry, attempt)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryDelay(retry *Retry, attempt int) time.Duration {
	if retry == nil || retry.Wait <= 0 {
		return 0
	}
	factor := retry.BackoffFactor
	if factor <= 0 {
		factor = 1
	}
	delay := float64(retry.Wait)
	for i := 0; i < attempt; i++ {
		delay *= factor
	}
	if retry.MaxWait > 0 && time.Duration(delay) > retry.MaxWait {
		return retry.MaxWait
	}
	return time.Duration(delay)
}

// buildEffectiveClient returns a client that respects per-request overrides.
func (s *Session) buildEffectiveClient(cfg *requestConfig) *http.Client {
	base := s.client
	if cfg.client != nil {
		base = cfg.client
	}

	client := cloneHTTPClient(base)
	if client.Jar == nil {
		client.Jar = s.Cookies
	}

	if cfg.roundTripper != nil {
		client.Transport = cfg.roundTripper
	} else if cfg.verify != nil || cfg.proxies != nil || cfg.transport != nil {
		t, ok := cloneHTTPTransport(client.Transport)
		if ok {
			if cfg.verify != nil {
				setInsecureSkipVerify(t, !*cfg.verify)
			}
			if cfg.proxies != nil {
				applyProxies(t, cfg.proxies)
			}
			if cfg.transport != nil {
				applyTransportConfig(t, cfg.transport)
			}
			client.Transport = t
		}
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}

	applyRedirectPolicy(client, s.AllowRedirects, cfg.allowRedirects, cfg.maxRedirects)
	return client
}

func cloneHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{}
	}
	c := *client
	return &c
}

func cloneHTTPTransport(roundTripper http.RoundTripper) (*http.Transport, bool) {
	if roundTripper == nil {
		return http.DefaultTransport.(*http.Transport).Clone(), true
	}
	t, ok := roundTripper.(*http.Transport)
	if !ok {
		return nil, false
	}
	return t.Clone(), true
}

func setInsecureSkipVerify(transport *http.Transport, insecure bool) {
	cfg := transport.TLSClientConfig
	if cfg == nil {
		cfg = &tls.Config{}
	} else {
		cfg = cfg.Clone()
	}
	cfg.InsecureSkipVerify = insecure //nolint:gosec // user-controlled setting
	transport.TLSClientConfig = cfg
}

func applyProxies(transport *http.Transport, proxies map[string]string) {
	if len(proxies) == 0 {
		return
	}
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if proxyURL, ok := proxies[req.URL.Scheme]; ok {
			return url.Parse(proxyURL)
		}
		return nil, nil
	}
}

func applyTransportConfig(transport *http.Transport, config *TransportConfig) {
	if config == nil {
		return
	}
	if config.MaxIdleConns > 0 {
		transport.MaxIdleConns = config.MaxIdleConns
	}
	if config.MaxIdleConnsPerHost > 0 {
		transport.MaxIdleConnsPerHost = config.MaxIdleConnsPerHost
	}
	if config.MaxConnsPerHost > 0 {
		transport.MaxConnsPerHost = config.MaxConnsPerHost
	}
	if config.IdleConnTimeout > 0 {
		transport.IdleConnTimeout = config.IdleConnTimeout
	}
	if config.TLSHandshakeTimeout > 0 {
		transport.TLSHandshakeTimeout = config.TLSHandshakeTimeout
	}
	if config.ResponseHeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = config.ResponseHeaderTimeout
	}
	if config.ExpectContinueTimeout > 0 {
		transport.ExpectContinueTimeout = config.ExpectContinueTimeout
	}
}

func applyRedirectPolicy(client *http.Client, sessionAllowRedirects bool, requestAllowRedirects *bool, requestMaxRedirects *int) {
	allowRedirects := sessionAllowRedirects
	if requestAllowRedirects != nil {
		allowRedirects = *requestAllowRedirects
	}
	if !allowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
		return
	}
	if requestMaxRedirects != nil {
		maxRedirects := *requestMaxRedirects
		if maxRedirects < 0 {
			maxRedirects = 0
		}
		client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		}
	}
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
	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	contentType := w.FormDataContentType()

	go func() {
		err := writeMultipart(w, cfg)
		if closeErr := w.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("go-requests: multipart close: %w", closeErr)
		}
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	return pr, contentType, nil
}

func writeMultipart(w *multipart.Writer, cfg *requestConfig) error {
	// Write form data fields first.
	if cfg.data != nil {
		for k, vals := range cfg.data {
			for _, v := range vals {
				if err := w.WriteField(k, v); err != nil {
					return fmt.Errorf("go-requests: multipart write field %q: %w", k, err)
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
			return fmt.Errorf("go-requests: multipart create file %q: %w", fieldName, err)
		}
		if _, err := io.Copy(fw, ff.Content); err != nil {
			return fmt.Errorf("go-requests: multipart copy file %q: %w", fieldName, err)
		}
	}

	return nil
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
