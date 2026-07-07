// sprinklerd is the sprinklerGo daemon: scheduling engine, REST API and
// embedded web interface in a single binary.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"sprinklergo/internal/api"
	"sprinklergo/internal/engine"
	"sprinklergo/internal/hardware"
	"sprinklergo/internal/model"
	"sprinklergo/internal/store"
	"sprinklergo/internal/weather"
	"sprinklergo/web"
)

// version is injected at build time via -ldflags "-X main.version=..."
// (see Makefile); "dev" identifies ad-hoc builds.
var version = "dev"

func main() {
	configPath := flag.String("config", "config.json", "path to the configuration file")
	dbPath := flag.String("db", "zonelog.db", "path to the zone log database")
	port := flag.Int("port", 0, "web port (overrides the configured setting)")
	debug := flag.Bool("debug", false, "enable debug logging")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if *debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	if err := run(*configPath, *dbPath, *port); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(configPath, dbPath string, portOverride int) error {
	cfg, err := store.OpenConfig(configPath)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	logs, err := store.OpenLog(dbPath)
	if err != nil {
		return fmt.Errorf("open zone log: %w", err)
	}
	defer logs.Close()

	settings := cfg.Snapshot().Settings

	out, err := hardware.ForSettings(settings)
	if err != nil {
		slog.Warn("hardware backend unavailable, outputs are NOT driven", "err", err)
		out = hardware.NewMock()
	}

	// The weather cache is refreshed hourly in the background; the engine
	// reads only the cached scale and never blocks on the network.
	wcache := weather.NewCache(func() model.Settings { return cfg.Snapshot().Settings })

	eng := engine.New(cfg, out, logs, wcache.Scale, nil)

	// applyOutput swaps the hardware backend when output settings change.
	var outMu sync.Mutex
	current := out
	applyOutput := func(s model.Settings) error {
		next, err := hardware.ForSettings(s)
		if err != nil {
			return err
		}
		outMu.Lock()
		defer outMu.Unlock()
		eng.SwapOutput(next)
		old := current
		current = next
		old.Apply(0)
		old.Close()
		slog.Info("output backend switched", "type", s.OutputType)
		return nil
	}

	staticFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}
	srv := api.New(version, cfg, logs, eng, wcache, staticFS, applyOutput)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	eng.Start(ctx)
	go wcache.Run(ctx, time.Hour)

	// Prune the zone log per retention setting, at startup and then daily.
	go func() {
		for {
			months := cfg.Snapshot().Settings.LogRetentionMonths
			if n, err := logs.Prune(months); err != nil {
				slog.Warn("zone log prune failed", "err", err)
			} else if n > 0 {
				slog.Info("pruned zone log", "rows", n, "retentionMonths", months)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(24 * time.Hour):
			}
		}
	}()

	addr := fmt.Sprintf(":%d", settings.WebPort)
	if portOverride != 0 {
		addr = fmt.Sprintf(":%d", portOverride)
	}
	httpSrv := &http.Server{Addr: addr, Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		httpSrv.Shutdown(shCtx)
	}()

	slog.Info("sprinklerd started", "version", version, "addr", addr, "config", configPath)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// Safety: make sure every valve is off before exiting.
	eng.Shutdown()
	outMu.Lock()
	current.Close()
	outMu.Unlock()
	slog.Info("sprinklerd stopped")
	return nil
}
