package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"

	"github.com/andreipimenov/golang-training-2021/internal/config"
	"github.com/andreipimenov/golang-training-2021/internal/handler"
	"github.com/andreipimenov/golang-training-2021/internal/repository"
	"github.com/andreipimenov/golang-training-2021/internal/service"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg, err := config.New()
	if err != nil {
		logger.Fatal().Err(err).Msg("Configuration error")
	}

	r := chi.NewRouter()

	client, err := mongo.NewClient(options.Client().ApplyURI(cfg.DBConnString))
	if err != nil {
		logger.Fatal().Err(err).Msg("DB initializing error")
	}

	err = client.Connect(context.TODO())
	if err != nil {
		logger.Fatal().Err(err).Msg("DB initializing error")
	}

	defer client.Disconnect(context.TODO())

	err = client.Ping(context.TODO(), nil)
	if err != nil {
		logger.Fatal().Err(err).Msg("DB pinging error")
	}

	// repo := repository.NewCache()
	//dbRepo := &repository.DB{DB: db}

	db := client.Database("backend")
	dbCollection := db.Collection("marketdata")
	dbRepo := repository.NewMongoDB(dbCollection)

	service := service.New(&logger, dbRepo, cfg.ExternalAPIToken)
	h := handler.New(&logger, service)

	r.Route("/", func(r chi.Router) {
		r.Use(middleware.RequestLogger(&handler.LogFormatter{Logger: &logger}))
		r.Use(middleware.Recoverer)
		r.Method(http.MethodGet, handler.Path, h)
	})

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdown)

	go func() {
		logger.Info().Msgf("Server is listening on :%d", cfg.Port)
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Server error")
		}
	}()

	<-shutdown

	logger.Info().Msg("Shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer func() {
		cancel()
	}()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Server shutdown error")
	}

	logger.Info().Msg("Server stopped gracefully")
}
