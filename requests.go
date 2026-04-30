package requests

import "net/http"

// defaultSession is a package-level Session used by the convenience functions.
// It behaves like a standard Session with no shared state configured.
var defaultSession = NewSession()

// Get sends a GET request and returns a Response.
//
//	resp, err := requests.Get("https://httpbin.org/get",
//	    requests.Params{"key": "value"},
//	)
func Get(url string, opts ...Option) (*Response, error) {
	return defaultSession.Get(url, opts...)
}

// Post sends a POST request and returns a Response.
//
//	resp, err := requests.Post("https://httpbin.org/post",
//	    requests.JSON{"key": "value"},
//	)
func Post(url string, opts ...Option) (*Response, error) {
	return defaultSession.Post(url, opts...)
}

// Put sends a PUT request and returns a Response.
func Put(url string, opts ...Option) (*Response, error) {
	return defaultSession.Put(url, opts...)
}

// Patch sends a PATCH request and returns a Response.
func Patch(url string, opts ...Option) (*Response, error) {
	return defaultSession.Patch(url, opts...)
}

// Delete sends a DELETE request and returns a Response.
func Delete(url string, opts ...Option) (*Response, error) {
	return defaultSession.Delete(url, opts...)
}

// Head sends a HEAD request and returns a Response.
func Head(url string, opts ...Option) (*Response, error) {
	return defaultSession.Head(url, opts...)
}

// Options sends an OPTIONS request and returns a Response.
func Options(url string, opts ...Option) (*Response, error) {
	return defaultSession.Options(url, opts...)
}

// Request sends an HTTP request with the given method and returns a Response.
func Request(method, url string, opts ...Option) (*Response, error) {
	return defaultSession.Request(method, url, opts...)
}

// newCookieJar creates a cookie jar used by sessions.
// It delegates to net/http's default implementation.
func newCookieJar() (http.CookieJar, error) {
	return cookieJarNew()
}
