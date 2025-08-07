package entrypoint

import (
	"fmt"
	"net/http"
	"os"

	"go.uber.org/zap"

	"github.com/sunr3d/05-08-2025/internal/api"
	"github.com/sunr3d/05-08-2025/internal/config"
	"github.com/sunr3d/05-08-2025/internal/infra/inmem"
	"github.com/sunr3d/05-08-2025/internal/middleware"
	"github.com/sunr3d/05-08-2025/internal/server"
	"github.com/sunr3d/05-08-2025/internal/services/archive_service"
)

func Run(cfg *config.Config, log *zap.Logger) error {

	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию для временных файлов: %w", err)
	} else {
		log.Info("директория для временных файлов создана", zap.String("path", cfg.TempDir))
	}
	if err := os.MkdirAll(cfg.ArchivesDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию для архивов: %w", err)
	} else {
		log.Info("директория для архивов создана", zap.String("path", cfg.ArchivesDir))
	}

	db := inmem.New(log, cfg.ArchiveTTL)
	svc := archive_service.New(log, cfg, db)
	controller := api.New(svc, log, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /archive", controller.CreateArchive)
	mux.HandleFunc("POST /archive/empty", controller.CreateEmptyArchive)
	mux.HandleFunc("POST /archive/add-file", controller.AddFile)
	mux.HandleFunc("GET /archive/status", controller.GetArchiveStatus)
	mux.HandleFunc("GET /download", controller.DownloadArchive)

	router := http.Handler(mux)
	router = middleware.JSONValidator()(router)
	router = middleware.ReqLogger(log)(router)
	router = middleware.Recovery(log)(router)

	srv := server.New(cfg.HTTPPort, router, log)
	return srv.Start()
}
