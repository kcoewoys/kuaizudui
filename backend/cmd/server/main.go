package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/eaok-cn/kuaizudui/backend/internal/config"
	"github.com/eaok-cn/kuaizudui/backend/internal/database"
	"github.com/eaok-cn/kuaizudui/backend/internal/httpapi"
	"github.com/eaok-cn/kuaizudui/backend/internal/platform"
	"github.com/redis/go-redis/v9"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to YAML configuration file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	if cfg.Server.Mode == "release" && cfg.Security.AdminTokenSecret == "change-me-in-production" {
		logger.Error("refusing release mode with the default admin token secret")
		os.Exit(1)
	}

	startupContext, cancelStartup := context.WithTimeout(context.Background(), cfg.Server.ReadTimeout.Value())
	defer cancelStartup()

	db, err := database.OpenMySQL(startupContext, cfg.MySQL, cfg.Server.Mode == "debug")
	if err != nil {
		logger.Error("mysql connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.Migrate(db); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Address, Password: cfg.Redis.Password, DB: cfg.Redis.Database,
		DialTimeout: cfg.Redis.DialTimeout.Value(), ReadTimeout: cfg.Redis.ReadTimeout.Value(),
		WriteTimeout: cfg.Redis.WriteTimeout.Value(),
	})
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.Ping(startupContext).Err(); err != nil {
		logger.Error("redis connection failed", "error", err)
		os.Exit(1)
	}

	application := platform.New(db, redisClient, cfg)
	if err := application.ResetActivityQueues(startupContext); err != nil {
		logger.Error("activity queue reset failed", "error", err)
		os.Exit(1)
	}

	resetContext, cancelReset := context.WithCancel(context.Background())
	defer cancelReset()
	go func() {
		if err := application.RunDailyResetScheduler(resetContext); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("daily reset scheduler stopped", "error", err)
		}
	}()

	router := httpapi.NewRouter(application, db, redisClient, cfg)
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: router, ReadTimeout: cfg.Server.ReadTimeout.Value(), WriteTimeout: cfg.Server.WriteTimeout.Value(),
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", server.Addr, "mode", cfg.Server.Mode)
		serverErrors <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case signal := <-signals:
		logger.Info("shutdown signal received", "signal", signal.String())
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
		}
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.Value())
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}

	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}
