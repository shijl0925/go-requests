package requests

import (
	"net/http"
	"testing"
)

func TestApplyDigestAuthWithQOPAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com/digest?x=1", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	applyDigestAuth(req, DigestAuth{Username: "user", Password: "pass"}, `Digest realm="test", nonce="nonce-value", qop="auth", opaque="opaque-value", algorithm=MD5`)

	header := req.Header.Get("Authorization")
	if header == "" {
		t.Fatal("expected Authorization header")
	}

	params := parseDigestChallenge(header)
	if params["username"] != "user" {
		t.Fatalf("unexpected username: %q", params["username"])
	}
	if params["realm"] != "test" {
		t.Fatalf("unexpected realm: %q", params["realm"])
	}
	if params["nonce"] != "nonce-value" {
		t.Fatalf("unexpected nonce: %q", params["nonce"])
	}
	if params["opaque"] != "opaque-value" {
		t.Fatalf("unexpected opaque: %q", params["opaque"])
	}
	if params["qop"] != "auth" {
		t.Fatalf("unexpected qop: %q", params["qop"])
	}
	if params["nc"] != "00000001" {
		t.Fatalf("unexpected nonce count: %q", params["nc"])
	}
	if params["cnonce"] == "" {
		t.Fatal("expected cnonce")
	}

	ha1 := md5sum("user:test:pass")
	ha2 := md5sum("GET:/digest?x=1")
	expectedResponse := md5sum(ha1 + ":nonce-value:00000001:" + params["cnonce"] + ":auth:" + ha2)
	if params["response"] != expectedResponse {
		t.Fatalf("unexpected digest response: got %q want %q", params["response"], expectedResponse)
	}
}

func TestParseDigestChallengeQuotedComma(t *testing.T) {
	params := parseDigestChallenge(`Digest realm="example, inc", nonce="nonce-value", qop="auth,auth-int", opaque="opaque,value", algorithm=MD5`)

	if params["realm"] != "example, inc" {
		t.Fatalf("unexpected realm: %q", params["realm"])
	}
	if params["nonce"] != "nonce-value" {
		t.Fatalf("unexpected nonce: %q", params["nonce"])
	}
	if params["qop"] != "auth,auth-int" {
		t.Fatalf("unexpected qop: %q", params["qop"])
	}
	if params["opaque"] != "opaque,value" {
		t.Fatalf("unexpected opaque: %q", params["opaque"])
	}
	if params["algorithm"] != "MD5" {
		t.Fatalf("unexpected algorithm: %q", params["algorithm"])
	}
}
