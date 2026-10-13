package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesConsole(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "ChainScope") {
		t.Fatal("response does not contain the console title")
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("content security policy is missing")
	}
}

func TestHandlerServesAssetsAndRejectsUnknownPaths(t *testing.T) {
	tests := []struct {
		path       string
		wantStatus int
		contains   string
	}{
		{path: "/assets/app.js", wantStatus: http.StatusOK, contains: "loadOverview"},
		{path: "/assets/styles.css", wantStatus: http.StatusOK, contains: "--background"},
		{path: "/missing", wantStatus: http.StatusNotFound, contains: "404"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			Handler().ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("response does not contain %q", test.contains)
			}
		})
	}
}
