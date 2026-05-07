package requests

import (
	"crypto/md5" //nolint:gosec // MD5 used for Digest auth per RFC 2617, not for cryptographic security
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
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

// DigestAuth implements HTTP Digest Authentication (RFC 2617) for MD5 with
// qop=auth.
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
//
// HTTP Digest Authentication (RFC 2617) mandates the use of MD5 as part of
// its challenge-response protocol. The MD5 usage here is dictated by the
// protocol specification and is not used for cryptographic password storage;
// the nonce provided by the server ensures replay protection.
func applyDigestAuth(req *http.Request, a DigestAuth, wwwAuthenticate string) error {
	params := parseDigestChallenge(wwwAuthenticate)
	realm := params["realm"]
	nonce := params["nonce"]
	algorithm := params["algorithm"]
	if algorithm == "" {
		algorithm = "MD5"
	}
	if !strings.EqualFold(algorithm, "MD5") {
		return fmt.Errorf("go-requests: unsupported digest auth algorithm %q", algorithm)
	}
	qop := selectDigestQOP(params["qop"])
	if params["qop"] != "" && qop == "" {
		return fmt.Errorf("go-requests: unsupported digest auth qop %q", params["qop"])
	}

	uri := req.URL.RequestURI()

	ha1 := md5sum(a.Username + ":" + realm + ":" + a.Password) //nolint:gosec
	ha2 := md5sum(req.Method + ":" + uri)                      //nolint:gosec
	response := md5sum(ha1 + ":" + nonce + ":" + ha2)          //nolint:gosec
	nonceCount := "00000001"
	cnonce := newDigestCNonce()
	if qop == "auth" {
		response = md5sum(ha1 + ":" + nonce + ":" + nonceCount + ":" + cnonce + ":" + qop + ":" + ha2) //nolint:gosec
	}

	header := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=%s, response="%s"`,
		a.Username, realm, nonce, uri, algorithm, response,
	)
	if params["opaque"] != "" {
		header += fmt.Sprintf(`, opaque="%s"`, params["opaque"])
	}
	if qop == "auth" {
		header += fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qop, nonceCount, cnonce)
	}
	req.Header.Set("Authorization", header)
	return nil
}

func md5sum(s string) string {
	h := md5.Sum([]byte(s)) //nolint:gosec
	return hex.EncodeToString(h[:])
}

func selectDigestQOP(qop string) string {
	for _, part := range strings.Split(qop, ",") {
		if strings.TrimSpace(part) == "auth" {
			return "auth"
		}
	}
	return ""
}

func newDigestCNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return md5sum(fmt.Sprint(time.Now().UnixNano())) //nolint:gosec
	}
	return hex.EncodeToString(b)
}

func parseDigestChallenge(header string) map[string]string {
	params := make(map[string]string)
	header = strings.TrimPrefix(header, "Digest ")
	for _, part := range splitDigestChallenge(header) {
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

func splitDigestChallenge(header string) []string {
	var parts []string
	start := 0
	inQuotes := false
	escaped := false
	for i, r := range header {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inQuotes:
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			parts = append(parts, header[start:i])
			start = i + 1
		}
	}
	parts = append(parts, header[start:])
	return parts
}
