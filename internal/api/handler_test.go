package api

import (
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinRoutesPreserveAPIAndWebConsole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(nil, Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Routes()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "web console", method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantBody: "ChainScope"},
		{name: "invalid block", method: http.MethodGet, path: "/api/v1/blocks/not-a-number", wantStatus: http.StatusBadRequest, wantBody: "invalid_block_number"},
		{name: "unknown API route", method: http.MethodGet, path: "/api/v1/unknown", wantStatus: http.StatusNotFound, wantBody: "route_not_found"},
		{name: "method not allowed", method: http.MethodPost, path: "/api/v1/network", wantStatus: http.StatusMethodNotAllowed, wantBody: "method_not_allowed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body does not contain %q: %s", test.wantBody, response.Body.String())
			}
		})
	}
}

func TestIsHash(t *testing.T) {
	valid := "0x0000000000000000000000000000000000000000000000000000000000000001"
	if !isHash(valid) {
		t.Fatal("expected valid hash")
	}
	for _, value := range []string{"", "0x1", "zz0000000000000000000000000000000000000000000000000000000000000000"} {
		if isHash(value) {
			t.Fatalf("expected invalid hash: %q", value)
		}
	}
}

func TestParseOptionalBlockNumber(t *testing.T) {
	for _, value := range []string{"", "latest"} {
		got, err := parseOptionalBlockNumber(value)
		if err != nil || got != nil {
			t.Fatalf("parse %q = %v, %v", value, got, err)
		}
	}
	got, err := parseOptionalBlockNumber("123")
	if err != nil || got.Cmp(big.NewInt(123)) != 0 {
		t.Fatalf("parse decimal = %v, %v", got, err)
	}
	if _, err := parseOptionalBlockNumber("0x7b"); err == nil {
		t.Fatal("expected hexadecimal input to be rejected")
	}
}

func TestWeiToEther(t *testing.T) {
	got := weiToEther(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	if got != "1.000000000000000000" {
		t.Fatalf("unexpected ether value: %s", got)
	}
}
