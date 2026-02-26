package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

type gzipReadCloser struct {
	reader *gzip.Reader
	body   io.Closer
}

func (grc *gzipReadCloser) Read(p []byte) (int, error) {
	return grc.reader.Read(p)
}

func (grc *gzipReadCloser) Close() error {
	err1 := grc.reader.Close()
	err2 := grc.body.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (grw *gzipResponseWriter) ensureGzip() {
	if grw.gz != nil {
		return
	}
	h := grw.Header()
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	h.Del("Content-Length")
	grw.gz = gzip.NewWriter(grw.ResponseWriter)
}

func (grw *gzipResponseWriter) WriteHeader(statusCode int) {
	if grw.wroteHeader {
		return
	}
	grw.wroteHeader = true

	// если у статуса нет тела, ничего сжимать не будем
	if statusCode == http.StatusNoContent || statusCode == http.StatusNotModified {
		grw.ResponseWriter.WriteHeader(statusCode)
		return
	}

	grw.ensureGzip()
	grw.ResponseWriter.WriteHeader(statusCode)
}

func (grw *gzipResponseWriter) Write(p []byte) (int, error) {
	if !grw.wroteHeader {
		grw.WriteHeader(http.StatusOK)
	}
	if grw.gz == nil {
		return grw.ResponseWriter.Write(p)
	}
	return grw.gz.Write(p)
}

func (grw *gzipResponseWriter) Flush() {
	if grw.gz != nil {
		_ = grw.gz.Flush()
	}
	if f, ok := grw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (grw *gzipResponseWriter) Close() error {
	if grw.gz != nil {
		return grw.gz.Close()
	}
	return nil
}

func hasToken(headerValue, token string) bool {
	for _, part := range strings.Split(headerValue, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 1) разархивирует body если content - gzip
		if hasToken(request.Header.Get("Content-Encoding"), "gzip") {
			gr, err := gzip.NewReader(request.Body)
			if err != nil {
				http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			request.Body = &gzipReadCloser{
				reader: gr,
				body:   request.Body,
			}
			request.Header.Del("Content-Encoding")
			request.ContentLength = -1
		}

		// 2) архивирует ответ если accept - gzip
		if !hasToken(request.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(writer, request)
			return
		}

		gw := &gzipResponseWriter{ResponseWriter: writer}
		defer func() {
			_ = gw.Close()
		}()

		next.ServeHTTP(gw, request)
	})
}
