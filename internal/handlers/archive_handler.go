package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/sunr3d/05-08-2025/internal/interfaces/services"
	"github.com/sunr3d/05-08-2025/models"
)

type ArchiveHandler struct {
	service services.ArchiveService
	logger  *zap.Logger
}

func New(service services.ArchiveService, logger *zap.Logger) *ArchiveHandler {
	return &ArchiveHandler{
		service: service,
		logger:  logger,
	}
}

// POST /archive
func (h *ArchiveHandler) CreateArchive(w http.ResponseWriter, r *http.Request) {
	var req createArchiveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("ошибка парсинга JSON запроса", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при парсинге JSON запроса", http.StatusInternalServerError)
		return
	}
	if len(req.URLs) < 1 || len(req.URLs) > 3 {
		http.Error(w, "Некорретный запрос: количество URL должно быть от 1 до 3", http.StatusBadRequest)
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
		ArchiveURL: fmt.Sprintf("/download/%s.zip", archive.ID),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("ошибка кодирования JSON ответа", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при кодировании JSON ответа", http.StatusInternalServerError)
	}
}

// POST /archive/empty
func (h *ArchiveHandler) CreateEmptyArchive(w http.ResponseWriter, r *http.Request) {
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

// POST /archive/{archive_id}/file
func (h *ArchiveHandler) AddFile(w http.ResponseWriter, r *http.Request) {
	archiveID := h.getArchiveID(r.URL.Path)
	if archiveID == "" {
		http.Error(w, "Некорректный ID архива в URL", http.StatusBadRequest)
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
		resp.Message = fmt.Sprintf("Файл успено добавлен к архиву \"%s\"", archiveID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("ошибка кодирования JSON ответа", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при кодировании JSON ответа", http.StatusInternalServerError)
	}
}

func (h *ArchiveHandler) GetArchiveStatus(w http.ResponseWriter, r *http.Request) {
	archiveID := h.getArchiveID(r.URL.Path)
	if archiveID == "" {
		http.Error(w, "Некорректный ID архива в URL", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	archive, err := h.service.GetArchiveStatus(ctx, archiveID)
	if err != nil {
		h.logger.Error("ошибка при попытке получения статуса архива", zap.Error(err))
		http.Error(w, err.Error(), http.StatusNotFound)
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
		resp.ArchiveURL = fmt.Sprintf("/download/%s.zip", archive.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("ошибка кодирования JSON ответа", zap.Error(err))
		http.Error(w, "Внутренняя ошибка сервера при кодировании JSON ответа", http.StatusInternalServerError)
	}
}

// Вспомогательные функции
func (h *ArchiveHandler) getArchiveID(path string) string {
	if !strings.HasPrefix(path, "/archive/") {
		return ""
	}

	path = strings.TrimPrefix(path, "/archive/")

	if strings.HasSuffix(path, "/file") {
		id := strings.TrimSuffix(path, "/file")
		return strings.Trim(id, "/")
	}

	if strings.HasSuffix(path, "/status") {
		id := strings.TrimSuffix(path, "/status")
		return strings.Trim(id, "/")
	}

	return ""
}
