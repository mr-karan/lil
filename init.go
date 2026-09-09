package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	flag "github.com/spf13/pflag"
)

func initConfig() {
	f := flag.NewFlagSet("config", flag.ContinueOnError)

	f.String("config", "config.toml", "path to config file")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Printf("Error parsing flags: %v", err)
		os.Exit(1)
	}

	// Load config file
	if err := ko.Load(file.Provider(f.Lookup("config").Value.String()), toml.Parser()); err != nil {
		log.Printf("Error loading config file: %v", err)
		os.Exit(1)
	}

	// Merge command-line flags over file configuration.
	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		log.Printf("Error loading environment variables: %v", err)
		os.Exit(1)
	}

	log.Println("Configuration loaded successfully")
}

func initLogger(debug bool) *slog.Logger {
	var level slog.Level
	if debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
