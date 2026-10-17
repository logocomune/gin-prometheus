package ginprom

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/quick"
)

func credentialHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func TestBasicAuthContract(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		allowed bool
	}{
		{name: "valid", header: credentialHeader("admin", "secret"), allowed: true},
		{name: "case insensitive scheme", header: "basic YWRtaW46c2VjcmV0", allowed: true},
		{name: "wrong username", header: credentialHeader("other", "secret")},
		{name: "wrong password", header: credentialHeader("admin", "wrong")},
		{name: "short password", header: credentialHeader("admin", "secre")},
		{name: "long password", header: credentialHeader("admin", "secret1")},
		{name: "short username", header: credentialHeader("admi", "secret")},
		{name: "long username", header: credentialHeader("admin1", "secret")},
		{name: "empty password", header: credentialHeader("admin", "")},
		{name: "empty username", header: credentialHeader("", "secret")},
		{name: "empty credentials", header: credentialHeader("", "")},
		{name: "missing header"},
		{name: "missing token", header: "Basic "},
		{name: "invalid base64", header: "Basic !!!"},
		{name: "missing colon", header: "Basic YWRtaW4="},
		{name: "wrong scheme", header: "Bearer YWRtaW46c2VjcmV0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !checkAuthentication(test.header, test.allowed) {
				t.Fatal("authentication status, challenge, or handler delegation changed")
			}
		})
	}
}

func checkAuthentication(header string, allowed bool) bool {
	called := false
	handler := withBasicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), "admin", "secret")
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", header)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if allowed {
		return called && response.Code == http.StatusNoContent && response.Header().Get("WWW-Authenticate") == ""
	}
	return !called && response.Code == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") == `Basic realm="restricted"` && response.Body.String() == "Unauthorized\n"
}

func TestBasicAuthCredentialProperty(t *testing.T) {
	property := func(username, password string) bool {
		return checkAuthentication(credentialHeader(username, password), username == "admin" && password == "secret")
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func FuzzBasicAuth(f *testing.F) {
	f.Add(credentialHeader("admin", "secret"))
	f.Add(credentialHeader("admin", "wrong"))
	f.Add("Basic !!!")
	f.Add("")
	f.Fuzz(func(t *testing.T, header string) {
		request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		request.Header.Set("Authorization", header)
		username, password, parsed := request.BasicAuth()
		allowed := parsed && username == "admin" && password == "secret"
		if !checkAuthentication(header, allowed) {
			t.Fatal("authentication contract violated")
		}
	})
}
