package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scribble-rs/scribble.rs/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRedirectHTTPToHTTPSRedirectsPlainHTTP(t *testing.T) {
	t.Parallel()

	handler := redirectHTTPToHTTPS(&config.Config{RootURL: "https://example.com"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/lobby/abc?room_auth=123", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusPermanentRedirect, recorder.Code)
	require.Equal(t, "https://example.com/lobby/abc?room_auth=123", recorder.Header().Get("Location"))
}

func TestRedirectHTTPToHTTPSUsesForwardedHost(t *testing.T) {
	t.Parallel()

	handler := redirectHTTPToHTTPS(&config.Config{RootURL: "https://fallback.example"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/resources/logo.png", nil)
	request.Header.Set("X-Forwarded-Host", "public.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusPermanentRedirect, recorder.Code)
	require.Equal(t, "https://public.example/resources/logo.png", recorder.Header().Get("Location"))
}

func TestRedirectHTTPToHTTPSAllowsForwardedHTTPS(t *testing.T) {
	t.Parallel()

	handler := redirectHTTPToHTTPS(&config.Config{RootURL: "https://example.com"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Empty(t, recorder.Header().Get("Location"))
}

func TestRedirectHTTPToHTTPSAllowsHealthCheck(t *testing.T) {
	t.Parallel()

	handler := redirectHTTPToHTTPS(&config.Config{RootURL: "https://example.com"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Empty(t, recorder.Header().Get("Location"))
}

func TestRedirectHTTPToHTTPSDisabledForHTTPRootURL(t *testing.T) {
	t.Parallel()

	handler := redirectHTTPToHTTPS(&config.Config{RootURL: "http://example.com"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Empty(t, recorder.Header().Get("Location"))
}
