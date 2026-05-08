package requests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Response wraps *http.Response and provides a more user-friendly API,
// similar to Python's requests.Response.
type Response struct {
	// StatusCode is the HTTP status code (e.g. 200, 404).
	StatusCode int

	// Status is the HTTP status line (e.g. "200 OK").
	Status string

	// Headers contains the response headers.
	Headers http.Header

	// Cookies contains the cookies set by the server.
	Cookies []*http.Cookie

	// URL is the final URL after any redirects.
	URL *url.URL

	// Request is the request that generated this response.
	Request *http.Request

	// RawResponse is the underlying *http.Response.
	RawResponse *http.Response

	// Elapsed is the time spent receiving the response headers and, for
	// non-streaming responses, reading the response body.
	Elapsed time.Duration

	// body caches the response body bytes after the first read.
	body []byte
	// bodyRead indicates whether the body has already been read.
	bodyRead bool
}

// newResponse creates a Response from a *http.Response. By default it reads and
// closes the response body so it is available for repeated access. In streaming
// mode, the caller is responsible for reading or closing the body.
func newResponse(r *http.Response, stream bool) (*Response, error) {
	resp := &Response{
		StatusCode:  r.StatusCode,
		Status:      r.Status,
		Headers:     r.Header,
		Cookies:     r.Cookies(),
		URL:         r.Request.URL,
		Request:     r.Request,
		RawResponse: r,
	}

	if stream {
		return resp, nil
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	resp.body = body
	resp.bodyRead = true
	return resp, nil
}

// Body returns the underlying response body for streaming reads.
// The caller must close the body when finished, or call Response.Close.
func (r *Response) Body() io.ReadCloser {
	if r.RawResponse == nil {
		return nil
	}
	return r.RawResponse.Body
}

// Close closes the underlying response body.
func (r *Response) Close() error {
	if r.RawResponse == nil || r.RawResponse.Body == nil {
		return nil
	}
	return r.RawResponse.Body.Close()
}

// ReadContent reads the response body into memory and caches it. It is useful
// for streamed responses when callers decide to consume the remaining body.
func (r *Response) ReadContent() ([]byte, error) {
	if r.bodyRead {
		return r.body, nil
	}
	if r.RawResponse == nil || r.RawResponse.Body == nil {
		r.bodyRead = true
		return r.body, nil
	}
	body, err := io.ReadAll(r.RawResponse.Body)
	if err != nil {
		return nil, err
	}
	_ = r.RawResponse.Body.Close()
	r.body = body
	r.bodyRead = true
	return r.body, nil
}

// Content returns the response body as a byte slice. Use ReadContent or JSON if
// callers need to observe body read errors.
func (r *Response) Content() []byte {
	if !r.bodyRead {
		body, err := r.ReadContent()
		if err == nil {
			return body
		}
	}
	return r.body
}

// Text returns the response body as a string. Use ReadContent or JSON if
// callers need to observe body read errors.
func (r *Response) Text() string {
	return string(r.Content())
}

// JSON unmarshals the response body into v.
// v must be a pointer to a value that can be unmarshalled from JSON.
func (r *Response) JSON(v interface{}) error {
	body, err := r.ReadContent()
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// JSONMap unmarshals the response body into a map.
func (r *Response) JSONMap() (map[string]interface{}, error) {
	var v map[string]interface{}
	err := r.JSON(&v)
	return v, err
}

// JSONSlice unmarshals the response body into a slice.
func (r *Response) JSONSlice() ([]interface{}, error) {
	var v []interface{}
	err := r.JSON(&v)
	return v, err
}

// SaveToFile writes the response body to path. For streaming responses this
// consumes and closes the underlying response body without caching it, so
// subsequent Content, Text, or JSON calls have no body left to read.
func (r *Response) SaveToFile(path string) error {
	if r.bodyRead {
		return os.WriteFile(path, r.body, 0o600)
	}

	if r.RawResponse == nil || r.RawResponse.Body == nil {
		return os.WriteFile(path, nil, 0o600)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r.RawResponse.Body)
	closeErr := r.RawResponse.Body.Close()
	r.bodyRead = true
	if err != nil {
		return err
	}
	return closeErr
}

// Ok returns true if the status code is less than 400.
func (r *Response) Ok() bool {
	return r.StatusCode < 400
}

// IsRedirect returns true if the status code is a redirect (3xx).
func (r *Response) IsRedirect() bool {
	return r.StatusCode >= 300 && r.StatusCode < 400
}

// Encoding attempts to detect the character encoding from the Content-Type header.
// It returns "utf-8" as the default when no explicit encoding is found.
func (r *Response) Encoding() string {
	ct := r.Headers.Get("Content-Type")
	for _, part := range strings.Split(ct, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "charset=") {
			return strings.TrimPrefix(strings.ToLower(part), "charset=")
		}
	}
	return "utf-8"
}

// RaiseForStatus returns an error if the status code represents a client or
// server error (4xx or 5xx), nil otherwise.
func (r *Response) RaiseForStatus() error {
	if r.StatusCode >= 400 {
		return &HTTPError{Response: r}
	}
	return nil
}

// HTTPError is returned by RaiseForStatus when the response indicates an error.
type HTTPError struct {
	Response *Response
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "go-requests: HTTP error"
	}
	if e.Response == nil {
		return "go-requests: HTTP error: response is nil"
	}
	status := e.Response.Status
	if status == "" && e.Response.StatusCode != 0 {
		status = fmt.Sprintf("%d", e.Response.StatusCode)
	}
	if status == "" {
		status = "unknown status"
	}
	if e.Response.URL == nil {
		return "go-requests: HTTP error: " + status
	}
	return status + ": " + e.Response.URL.String()
}
