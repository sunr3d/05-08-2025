package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/sunr3d/05-08-2025/internal/config"
	"github.com/sunr3d/05-08-2025/internal/interfaces/services"
	"github.com/sunr3d/05-08-2025/models"
)

type ArchiveAPI struct {
	service services.ArchiveService
	logger  *zap.Logger
	cfg     *config.Config
}

func New(service services.ArchiveService, logger *zap.Logger, cfg *config.Config) *ArchiveAPI {
	return &ArchiveAPI{
		service: service,
		logger:  logger,
		cfg:     cfg,
	}
}

// POST /archive
func (h *ArchiveAPI) CreateArchive(w http.ResponseWriter, r *http.Request) {
	var req createArchiveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("ошибка парсинга JSON запроса", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при парсинге JSON запроса", http.StatusInternalServerError)
		return
	}
	if len(req.URLs) < 1 || len(req.URLs) > 3 {
		http.Error(w, "Некорректный запрос: количество URL должно быть от 1 до 3", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	archive, err := h.service.CreateArchive(ctx, req.URLs)
	if err != nil {
		h.logger.Error("ошибка создания архива", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := createArchiveResp{
		ID:         archive.ID,
		Status:     string(archive.Status),
		Files:      archive.Files,
		Errors:     archive.Errors,
		CreatedAt:  archive.CreatedAt.Format(time.RFC3339),
		ArchiveURL: fmt.Sprintf("/download?archive_id=%s", archive.ID),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("ошибка кодирования JSON ответа", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при кодировании JSON ответа", http.StatusInternalServerError)
	}
}

// POST /archive/empty
func (h *ArchiveAPI) CreateEmptyArchive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	archive, err := h.service.CreateEmptyArchive(ctx)
	if err != nil {
		h.logger.Error("ошибка создания пустого архива", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := createEmptyArchiveResp{
		ID:        archive.ID,
		Status:    string(archive.Status),
		CreatedAt: archive.CreatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("ошибка кодирования JSON ответа", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при кодировании JSON ответа", http.StatusInternalServerError)
	}
}

// POST /archive/add-file?archive_id={archive_id}
func (h *ArchiveAPI) AddFile(w http.ResponseWriter, r *http.Request) {
	archiveID := r.URL.Query().Get("archive_id")
	if archiveID == "" {
		http.Error(w, "Некорректный запрос: отсутствует archive_id", http.StatusBadRequest)
		return
	}

	var req addFileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("ошибка парсинга JSON запроса", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при парсинге JSON запроса", http.StatusInternalServerError)
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		http.Error(w, "Некорректный запрос: поле URL не может быть пустым", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	resp := addFileResp{}
	err := h.service.AddFile(ctx, archiveID, req.URL)
	if err != nil {
		h.logger.Error("ошибка при добавлении файла в архив", zap.Error(err))
		resp.Success = false
		resp.Message = err.Error()
	} else {
		resp.Success = true
		resp.Message = fmt.Sprintf("Файл успешно добавлен к архиву \"%s\"", archiveID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("ошибка кодирования JSON ответа", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при кодировании JSON ответа", http.StatusInternalServerError)
	}
}

// GET /archive/status?archive_id={archive_id}
func (h *ArchiveAPI) GetArchiveStatus(w http.ResponseWriter, r *http.Request) {
	archiveID := r.URL.Query().Get("archive_id")
	if archiveID == "" {
		http.Error(w, "Некорректный запрос: отсутствует archive_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	archive, err := h.service.GetArchive(ctx, archiveID)
	if err != nil {
		h.logger.Error("ошибка при попытке получения статуса архива", zap.Error(err))
		http.Error(w, "Архив не найден", http.StatusNotFound)
		return
	}

	resp := getArchiveStatusResp{
		ID:        archive.ID,
		Status:    string(archive.Status),
		Files:     archive.Files,
		Errors:    archive.Errors,
		CreatedAt: archive.CreatedAt.Format(time.RFC3339),
		UpdatedAt: archive.UpdatedAt.Format(time.RFC3339),
	}

	if archive.Status == models.ArchiveStatusReady {
		resp.ArchiveURL = fmt.Sprintf("/download?archive_id=%s", archive.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("ошибка кодирования JSON ответа", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при кодировании JSON ответа", http.StatusInternalServerError)
	}
}

// GET /download?archive_id={archive_id}
func (h *ArchiveAPI) DownloadArchive(w http.ResponseWriter, r *http.Request) {
	archiveID := r.URL.Query().Get("archive_id")
	if archiveID == "" {
		http.Error(w, "Некорректный запрос: отсутствует archive_id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	archive, err := h.service.GetArchive(ctx, archiveID)
	if err != nil {
		h.logger.Error("ошибка при попытке получения статуса архива", zap.Error(err))
		http.Error(w, "Архив не найден", http.StatusNotFound)
		return
	}

	if archive.Status != models.ArchiveStatusReady {
		http.Error(w, "Архив недоступен для скачивания", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(h.cfg.ArchivesDir, archiveID+".zip")

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.zip\"", archiveID))

	http.ServeFile(w, r, filePath)
}
