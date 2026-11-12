package manual

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func resultRequest(method, path, host, origin, body string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), method, "http://"+host+path, strings.NewReader(body))
	req.Host = host
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestCaptchaResultHandler(t *testing.T) {
	const path = "/local-captcha-result/test-secret"
	origin := localCaptchaOrigin()
	host := strings.TrimPrefix(origin, "http://")
	form := "token=" + url.QueryEscape("success-token")

	tests := []struct {
		name      string
		method    string
		path      string
		host      string
		origin    string
		body      string
		wantCode  int
		wantToken bool
	}{
		{"accepts matching request", http.MethodPost, path, host, origin, form, http.StatusOK, true},
		{"wrong secret path", http.MethodPost, "/local-captcha-result/other", host, origin, form, http.StatusNotFound, false},
		{"wrong origin", http.MethodPost, path, host, "http://evil.example", form, http.StatusNotFound, false},
		{"missing origin", http.MethodPost, path, host, "", form, http.StatusNotFound, false},
		{"wrong host", http.MethodPost, path, "evil.example", origin, form, http.StatusNotFound, false},
		{"GET not allowed", http.MethodGet, path, host, origin, "", http.StatusMethodNotAllowed, false},
		{"empty token", http.MethodPost, path, host, origin, "token=", http.StatusBadRequest, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyCh := make(chan string, 1)
			rec := httptest.NewRecorder()
			captchaResultHandler(path, keyCh).ServeHTTP(rec, resultRequest(tc.method, tc.path, tc.host, tc.origin, tc.body))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("Access-Control-Allow-Origin = %q, want none", got)
			}
			select {
			case got := <-keyCh:
				if !tc.wantToken || got != "success-token" {
					t.Fatalf("unexpected token %q (wantToken=%v)", got, tc.wantToken)
				}
			default:
				if tc.wantToken {
					t.Fatal("token was not delivered")
				}
			}
		})
	}
}
