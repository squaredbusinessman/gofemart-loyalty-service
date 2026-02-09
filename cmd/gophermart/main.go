package main

import (
	"fmt"
	"os"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/config"
)

func main() {
	// грузим кофиг
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// валидируем загруженный конфиг
	if err := cfg.Validate(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
