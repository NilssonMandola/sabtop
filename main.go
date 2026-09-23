// Command sabtop is a terminal console for a SABnzbd server: the download
// queue, history, disk headroom and news-server stats.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/NilssonMandola/sabtop/internal/config"
	"github.com/NilssonMandola/sabtop/internal/sab"
	"github.com/NilssonMandola/sabtop/internal/ui"
)

// version is overwritten at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sabtop:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		flagURL     = flag.String("url", "", "SABnzbd URL (default "+config.DefaultURL+")")
		flagKey     = flag.String("key", "", "SABnzbd API key")
		flagVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *flagVersion {
		fmt.Println("sabtop", version)
		return nil
	}

	cfg, err := config.Load(*flagURL, *flagKey)
	if err != nil {
		if errors.Is(err, config.ErrNoToken) {
			return errors.New("no API key configured\n\n" + config.SetupHint())
		}
		return err
	}

	client := sab.New(cfg.URL, cfg.Token)

	// Fail before entering the alt screen, so errors stay readable.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sabVersion, err := client.Version(ctx)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", cfg.URL, err)
	}

	prog := tea.NewProgram(ui.New(client, sabVersion, cfg.URL), tea.WithAltScreen())
	_, err = prog.Run()
	return err
}

func usage() {
	fmt.Fprintf(os.Stderr, `sabtop — terminal console for SABnzbd

Usage:
  sabtop [flags]

Flags:
  -url string   SABnzbd URL (default %s)
  -key string   SABnzbd API key
  -version      print version and exit

Configuration is read from flags, then $SAB_URL / $SAB_API_KEY,
then %s.
`, config.DefaultURL, config.Path())
}
