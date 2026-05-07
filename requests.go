package requests

import (
	"net/http"
	"sync"
)

// defaultSession is a package-level Session used by the convenience functions.
// It behaves like a standard Session and shares cookies and other session state
// globally across all package-level convenience function calls, including from
// concurrent goroutines.
// Access and replacement are safe for concurrent callers.
// Use Session setter methods for concurrent-safe mutations of the returned
// default session; direct field mutations are not concurrency-safe.
var (
	defaultSessionMu sync.RWMutex
	defaultSession   = NewSession()
)

// DefaultSession returns the package-level Session used by the convenience
// functions. Mutating it affects subsequent package-level requests.
func DefaultSession() *Session {
	defaultSessionMu.RLock()
	defer defaultSessionMu.RUnlock()
	return defaultSession
}

// ResetDefaultSession replaces the package-level Session used by the
// convenience functions and returns the new Session.
func ResetDefaultSession() *Session {
	defaultSessionMu.Lock()
	defer defaultSessionMu.Unlock()
	defaultSession = NewSession()
	return defaultSession
}

// Get sends a GET request and returns a Response.
//
//	resp, err := requests.Get("https://httpbin.org/get",
//	    requests.Params{"key": "value"},
//	)
func Get(url string, opts ...Option) (*Response, error) {
	return DefaultSession().Get(url, opts...)
}

// Post sends a POST request and returns a Response.
//
//	resp, err := requests.Post("https://httpbin.org/post",
//	    requests.JSON{"key": "value"},
//	)
func Post(url string, opts ...Option) (*Response, error) {
	return DefaultSession().Post(url, opts...)
}

// Put sends a PUT request and returns a Response.
func Put(url string, opts ...Option) (*Response, error) {
	return DefaultSession().Put(url, opts...)
}

// Patch sends a PATCH request and returns a Response.
func Patch(url string, opts ...Option) (*Response, error) {
	return DefaultSession().Patch(url, opts...)
}

// Delete sends a DELETE request and returns a Response.
func Delete(url string, opts ...Option) (*Response, error) {
	return DefaultSession().Delete(url, opts...)
}

// Head sends a HEAD request and returns a Response.
func Head(url string, opts ...Option) (*Response, error) {
	return DefaultSession().Head(url, opts...)
}

// Options sends an OPTIONS request and returns a Response.
func Options(url string, opts ...Option) (*Response, error) {
	return DefaultSession().Options(url, opts...)
}

// Request sends an HTTP request with the given method and returns a Response.
func Request(method, url string, opts ...Option) (*Response, error) {
	return DefaultSession().Request(method, url, opts...)
}

// newCookieJar creates a cookie jar used by sessions.
// It delegates to net/http's default implementation.
func newCookieJar() (http.CookieJar, error) {
	return cookieJarNew()
}
