package requests

import (
	"net/http"
	"net/http/cookiejar"
)

// cookieJarNew creates a new cookie jar.
func cookieJarNew() (http.CookieJar, error) {
	return cookiejar.New(nil)
}
