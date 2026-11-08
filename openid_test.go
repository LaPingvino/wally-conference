package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A homeserver answering "sub" with subFor(its own server name).
func fakeHomeserver(t *testing.T, subFor func(serverName string) string) (serverName string) {
	t.Helper()
	var name string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `{"sub":%q}`, subFor(name))
	}))
	name = strings.TrimPrefix(srv.URL, "http://")
	t.Cleanup(srv.Close)
	orig := verifyOpenIDURL
	verifyOpenIDURL = func(serverName, accessToken string) string {
		return srv.URL + "/_matrix/federation/v1/openid/userinfo?access_token=" + accessToken
	}
	t.Cleanup(func() { verifyOpenIDURL = orig })
	return name
}

func TestVerifyOpenIDAcceptsOwnUsers(t *testing.T) {
	name := fakeHomeserver(t, func(n string) string { return "@alice:" + n })
	got, err := VerifyOpenID(context.Background(), OpenIDToken{AccessToken: "tok", MatrixServerName: name})
	if err != nil || got.String() != "@alice:"+name {
		t.Fatalf("got %q, %v; want @alice:%s", got, err, name)
	}
}

func TestVerifyOpenIDRejectsForeignClaims(t *testing.T) {
	// evil.example's server vouching for a kiefte.eu user: the forged-identity hole.
	name := fakeHomeserver(t, func(string) string { return "@joop:kiefte.eu" })
	if _, err := VerifyOpenID(context.Background(), OpenIDToken{AccessToken: "tok", MatrixServerName: name}); err == nil {
		t.Fatal("a server vouched for another server's user and was believed")
	}
}

func TestVerifyOpenIDRejectsBadServerNames(t *testing.T) {
	for _, bad := range []string{"", "evil.example/path", "user@evil.example", "evil.example?x=1", "evil.example#f"} {
		if _, err := VerifyOpenID(context.Background(), OpenIDToken{AccessToken: "tok", MatrixServerName: bad}); err == nil {
			t.Errorf("server name %q accepted", bad)
		}
	}
}

func TestVerifyOpenIDRejectsBadToken(t *testing.T) {
	name := fakeHomeserver(t, func(n string) string { return "@alice:" + n })
	if _, err := VerifyOpenID(context.Background(), OpenIDToken{AccessToken: "wrong", MatrixServerName: name}); err == nil {
		t.Fatal("rejected token accepted")
	}
}

func TestDebugEndpointsNeedTheToken(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }
	for _, c := range []struct {
		token, header string
		want          int
	}{
		{"", "", http.StatusNotFound},              // no token configured: off
		{"", "Bearer ", http.StatusNotFound},       // empty never matches
		{"s3cret", "", http.StatusNotFound},        // anonymous
		{"s3cret", "Bearer nope", http.StatusNotFound},
		{"s3cret", "Bearer s3cret", http.StatusTeapot},
	} {
		svc := &Service{Config: &Config{DebugToken: c.token}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/guest/debug/x", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		svc.requireDebugToken(ok)(rec, req)
		if rec.Code != c.want {
			t.Errorf("token %q header %q: got %d, want %d", c.token, c.header, rec.Code, c.want)
		}
	}
}
