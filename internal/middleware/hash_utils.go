package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

func sha256hex(body []byte, key string) string {
	sum := sha256.Sum256(append(body, []byte(key)...))
	return hex.EncodeToString(sum[:])
}

type ResponseRecorder struct {
	header      http.Header
	status      int
	body        bytes.Buffer
	wroteHeader bool
}

func NewRecorder(_ http.ResponseWriter) *ResponseRecorder {
	return &ResponseRecorder{
		header: make(http.Header),
	}
}

func (r *ResponseRecorder) Header() http.Header {
	return r.header
}

func (r *ResponseRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.status = statusCode
	r.wroteHeader = true
}

func (r *ResponseRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(p)
}

func (r *ResponseRecorder) Body() []byte {
	return r.body.Bytes()
}

func (r *ResponseRecorder) FlushTo(w http.ResponseWriter) {
	for k, vv := range r.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	if r.status == 0 {
		r.status = http.StatusOK
	}
	w.WriteHeader(r.status)

	if r.body.Len() > 0 {
		_, _ = w.Write(r.body.Bytes())
	}
}
