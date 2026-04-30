package requests

import (
	"crypto/md5" //nolint:gosec // MD5 used for Digest auth per RFC 2617, not for cryptographic security
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// AuthProvider is implemented by any value that can add authentication
// information to an HTTP request.
type AuthProvider interface {
	// Apply sets the relevant headers or fields on the request.
	Apply(req *http.Request)
}

// BasicAuth implements HTTP Basic Authentication (RFC 7617).
type BasicAuth struct {
	Username string
	Password string
}

// Apply sets the Authorization header using HTTP Basic Authentication.
func (a BasicAuth) Apply(req *http.Request) {
	req.SetBasicAuth(a.Username, a.Password)
}

// TokenAuth implements Bearer token authentication.
type TokenAuth struct {
	Token string
}

// Apply sets the Authorization header to "Bearer <token>".
func (a TokenAuth) Apply(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.Token)
}

// DigestAuth implements HTTP Digest Authentication (RFC 2617).
// Note: this is a simplified single-round implementation and does not support
// qop="auth" nonce counters or opaque values beyond a single exchange.
type DigestAuth struct {
	Username string
	Password string
}

// Apply is a no-op at request-build time; digest auth requires the server's
// WWW-Authenticate challenge. Use Session.SetAuth or pass DigestAuth via
// WithAuth — the Session will automatically retry on 401 to complete the
// handshake.
func (a DigestAuth) Apply(_ *http.Request) {}

// applyDigestAuth performs the second step of Digest auth using the challenge
// contained in the 401 response header.
// applyDigestAuth performs the second step of Digest auth using the challenge
// contained in the 401 response header.
//
// HTTP Digest Authentication (RFC 2617) mandates the use of MD5 as part of
// its challenge-response protocol. The MD5 usage here is dictated by the
// protocol specification and is not used for cryptographic password storage;
// the nonce provided by the server ensures replay protection.
func applyDigestAuth(req *http.Request, a DigestAuth, wwwAuthenticate string) {
	params := parseDigestChallenge(wwwAuthenticate)
	realm := params["realm"]
	nonce := params["nonce"]
	algorithm := params["algorithm"]
	if algorithm == "" {
		algorithm = "MD5"
	}

	uri := req.URL.RequestURI()

	ha1 := md5sum(a.Username + ":" + realm + ":" + a.Password) //nolint:gosec
	ha2 := md5sum(req.Method + ":" + uri)                       //nolint:gosec
	response := md5sum(ha1 + ":" + nonce + ":" + ha2)           //nolint:gosec

	header := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=%s, response="%s"`,
		a.Username, realm, nonce, uri, algorithm, response,
	)
	req.Header.Set("Authorization", header)
}

func md5sum(s string) string {
	h := md5.Sum([]byte(s)) //nolint:gosec
	return hex.EncodeToString(h[:])
}

func parseDigestChallenge(header string) map[string]string {
	params := make(map[string]string)
	header = strings.TrimPrefix(header, "Digest ")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		params[key] = value
	}
	return params
}
