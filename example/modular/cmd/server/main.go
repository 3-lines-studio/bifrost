package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/3-lines-studio/bifrost"

	"github.com/3-lines-studio/bifrost/example/modular/internal/app"
	"github.com/3-lines-studio/bifrost/example/modular/internal/billing"
	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
	"github.com/3-lines-studio/bifrost/example/modular/internal/db"
	"github.com/3-lines-studio/bifrost/example/modular/internal/i18n"
	"github.com/3-lines-studio/bifrost/example/modular/internal/mailer"
	"github.com/3-lines-studio/bifrost/example/modular/internal/notify"
	"github.com/3-lines-studio/bifrost/example/modular/internal/queue"
	"github.com/3-lines-studio/bifrost/example/modular/internal/storage"
	"github.com/3-lines-studio/bifrost/example/modular/internal/user"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// 1. instance every module with no dependencies.
	cfg := config.New()
	database := db.New()
	taskQueue := queue.New()
	files := storage.New()
	sender := mailer.New()
	catalog := i18n.New()
	userModule := user.New()
	billingModule := billing.New()
	notifyModule := notify.New()

	// 2. wire leaves first, then the modules that depend on them.
	cfg.Wire()
	database.Wire(ctx, cfg)
	taskQueue.Wire(cfg)
	files.Wire(cfg)
	sender.Wire(cfg)
	catalog.Wire(cfg)

	userModule.Wire(database, catalog)
	billingModule.Wire(database, taskQueue, files, catalog, userModule)
	notifyModule.Wire(taskQueue, sender, catalog)

	// 3. wire the app, mounting the HTTP and asynq muxes.
	web, err := app.New(cfg, bifrostAssets, logger, userModule, billingModule, notifyModule)
	if err != nil {
		logger.Error("wire", "err", err)
		os.Exit(1)
	}

	// 4. build-phase guard: declarations are already collected above; no
	// listener has been opened yet, so describe/generate returns cleanly.
	if bifrost.Building() {
		return
	}

	// 5. run HTTP plus the asynq server and module background loops.
	if err := web.Run(ctx); err != nil {
		logger.Error("run", "err", err)
		os.Exit(1)
	}
}
