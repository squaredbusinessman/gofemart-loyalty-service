package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoverer_PanicReturns500(t *testing.T) {
	t.Parallel()

	h := Recoverer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
	require.Equal(t, "Internal Server Error\n", res.Body.String())
}

func TestRecoverer_NoPanicPassThrough(t *testing.T) {
	t.Parallel()

	h := Recoverer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusTeapot)
		_, _ = writer.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusTeapot, res.Code)
	require.Equal(t, "ok", res.Body.String())
}
