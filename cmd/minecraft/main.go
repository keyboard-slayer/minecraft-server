package main

import (
	"fmt"
	"os"

	"log/slog"

	"github.com/charmbracelet/log"
	"github.com/keyboard-slayer/minecraft-server/internal/minecraft"
)

func main() {
	handler := log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller: true,
		Level:        log.DebugLevel,
		Prefix:       "Server",
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	serv, err := minecraft.New(6969)
	if err != nil {
		fmt.Println("Error starting server: ", err)
		os.Exit(1)
	}

	serv.Serve()
}
