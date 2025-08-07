package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

const (
	shutdownTimeout = 5 * time.Second
	httpTimeout     = 10 * time.Second
)

type Server struct {
	server *http.Server
	logger *zap.Logger
}

func New(port string, handler http.Handler, logger *zap.Logger) *Server {
	return &Server{
		server: &http.Server{
			Addr:         "localhost:" + port,
			Handler:      handler,
			ReadTimeout:  httpTimeout,
			WriteTimeout: httpTimeout,
		},
		logger: logger,
	}
}

func (s *Server) Start() error {
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)

	go func() {
		s.logger.Info("Запуск HTTP сервера", zap.String("address", s.server.Addr))
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("ошибка HTTP сервера: %w", err)
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case sig := <-done:
		s.logger.Info("Получен сигнал завершения", zap.String("signal", sig.String()))

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("ошибка при завершении сервера: %w", err)
		}

		s.logger.Info("HTTP сервер успешно остановлен")
		return nil
	}
}
