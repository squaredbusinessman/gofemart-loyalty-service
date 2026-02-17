package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func gunzipBytes(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	require.NoError(t, err)
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	require.NoError(t, err)
	return string(raw)
}

func TestGzip_DecompressRequest(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "hello", string(body))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(gzipBytes(t, "hello")))
	req.Header.Set("Content-Encoding", "gzip")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
}

func TestGzip_InvalidRequestBodyReturns400(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-gzip"))
	req.Header.Set("Content-Encoding", "gzip")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestGzip_CompressResponse(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "gzip", res.Header().Get("Content-Encoding"))
	require.Contains(t, res.Header().Get("Vary"), "Accept-Encoding")
	require.JSONEq(t, `{"ok":true}`, gunzipBytes(t, res.Body.Bytes()))
}
