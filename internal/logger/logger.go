package logger

import (
	"net/http"

	"go.uber.org/zap"
)

type LoggingWriter struct {
	http.ResponseWriter
	Status int
	Bytes  int
}

func (lw *LoggingWriter) WriteHeader(code int) {
	lw.Status = code
	lw.ResponseWriter.WriteHeader(code)
}

func (lw *LoggingWriter) Write(b []byte) (int, error) {
	n, err := lw.ResponseWriter.Write(b)
	lw.Bytes += n
	if lw.Status == 0 {
		lw.Status = http.StatusOK
	}
	return n, err
}

var Log *zap.Logger = zap.NewNop()

func Initialize(level string) error {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return err
	}

	cfg := zap.NewProductionConfig()

	cfg.Level = lvl

	zapLog, err := cfg.Build()
	if err != nil {
		return err
	}

	Log = zapLog
	return nil
}
