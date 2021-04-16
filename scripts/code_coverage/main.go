package main

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/go-github/v35/github"
	"github.com/jacobweinstock/goconfig"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/oauth2"
)

type configuration struct {
	LogLevel    string `flag:"loglevel"`
	Config      string
	AccessToken string `flag:"accesstoken"`
	ID          string
	Filename    string
	Content     string
}

func main() {
	// set default config values
	config := configuration{
		LogLevel: "info",
	}

	cfgParser := goconfig.NewParser(
		goconfig.WithPrefix("GISTEDIT"),
		goconfig.WithFile("gistedit.yaml"),
	)
	err := cfgParser.Parse(&config)
	if err != nil {
		panic(err)
	}

	log := defaultLogger(config.LogLevel)
	err = run(config.AccessToken, config.ID, config.Filename, config.Content)
	if err != nil {
		log.V(0).Error(err, "failed to update gist")
		os.Exit(1)
	}

}

func run(accessToken, gistID, filename, content string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	gist, _, err := client.Gists.Get(ctx, gistID)
	if err != nil {
		return errors.Wrapf(err, "error getting gist: %v", gistID)
	}
	for fname, fileDetails := range gist.Files {
		if fname == github.GistFilename(filename) {
			newContent := fileDetails
			newContent.Content = &content
			gist.Files[fname] = newContent
			_, _, err := client.Gists.Edit(ctx, gistID, gist)
			if err != nil {
				return errors.Wrapf(err, "error updating gist: %v", gistID)
			}
			break
		}
	}

	return nil
}

// defaultLogger is zap logr implementation
func defaultLogger(level string) logr.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	var zLevel zapcore.Level
	switch level {
	case "debug":
		zLevel = zapcore.DebugLevel
	default:
		zLevel = zapcore.InfoLevel
	}
	config.Level = zap.NewAtomicLevelAt(zLevel)
	zapLogger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("who watches the watchmen (%v)?", err))
	}

	return zapr.NewLogger(zapLogger)
}
