package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/sunr3d/05-08-2025/internal/config"
	"github.com/sunr3d/05-08-2025/internal/infra/inmem"
	"github.com/sunr3d/05-08-2025/internal/services/archive_service"
)

func setupTestAPI(t *testing.T) (*ArchiveAPI, func()) {
	logger := zaptest.NewLogger(t)

	testDir := filepath.Join(os.TempDir(), "api_test_"+time.Now().Format("20060102150405"))
	archivesDir := filepath.Join(testDir, "archives")
	tempDir := filepath.Join(testDir, "temp")

	os.MkdirAll(archivesDir, 0755)
	os.MkdirAll(tempDir, 0755)

	cfg := &config.Config{
		HTTPTimeout:          30 * time.Second,
		AllowedExtensions:    []string{"application/pdf", "image/jpeg", "image/jpg"},
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		ArchiveTTL:           1 * time.Hour,
		ArchivesDir:          archivesDir,
		TempDir:              tempDir,
	}

	repo := inmem.New(logger, cfg.ArchiveTTL)
	service := archive_service.New(logger, cfg, repo)
	api := New(service, logger, cfg)

	cleanup := func() {
		os.RemoveAll(testDir)
	}

	return api, cleanup
}

func TestArchiveAPI_CreateArchive_Success(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := createArchiveReq{
		URLs: []string{
			"https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf",
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.CreateArchive(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp createArchiveResp
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "ready", resp.Status)
	assert.Len(t, resp.Files, 1)
	assert.Empty(t, resp.Errors)
	assert.NotEmpty(t, resp.CreatedAt)
}

func TestArchiveAPI_CreateArchive_InvalidJSON(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/archive", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.CreateArchive(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Внутренняя ошибка сервера при парсинге JSON запроса")
}

func TestArchiveAPI_CreateArchive_TooManyURLs(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := createArchiveReq{
		URLs: []string{"url1", "url2", "url3", "url4"},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.CreateArchive(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "количество URL должно быть от 1 до 3")
}

func TestArchiveAPI_CreateArchive_EmptyURLs(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := createArchiveReq{
		URLs: []string{},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.CreateArchive(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "количество URL должно быть от 1 до 3")
}

func TestArchiveAPI_CreateEmptyArchive_Success(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/archive/empty", nil)
	w := httptest.NewRecorder()

	api.CreateEmptyArchive(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp createEmptyArchiveResp
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "empty", resp.Status)
	assert.NotEmpty(t, resp.CreatedAt)
}

func TestArchiveAPI_AddFile_Success(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/archive/empty", nil)
	w := httptest.NewRecorder()
	api.CreateEmptyArchive(w, req)

	var resp createEmptyArchiveResp
	json.Unmarshal(w.Body.Bytes(), &resp)

	reqBody := addFileReq{
		URL: "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf",
	}

	body, _ := json.Marshal(reqBody)
	req = httptest.NewRequest(http.MethodPost, "/archive/add-file?archive_id="+resp.ID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	api.AddFile(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var addResp addFileResp
	err := json.Unmarshal(w.Body.Bytes(), &addResp)
	require.NoError(t, err)

	assert.True(t, addResp.Success)
	assert.Contains(t, addResp.Message, "Файл успешно добавлен")
}

func TestArchiveAPI_AddFile_MissingArchiveID(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := addFileReq{
		URL: "https://example.com/file.pdf",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive/add-file", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.AddFile(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "отсутствует archive_id")
}

func TestArchiveAPI_AddFile_EmptyURL(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := addFileReq{
		URL: "",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive/add-file?archive_id=test", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.AddFile(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "поле URL не может быть пустым")
}

func TestArchiveAPI_AddFile_InvalidJSON(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/archive/add-file?archive_id=test", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.AddFile(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Внутренняя ошибка сервера при парсинге JSON запроса")
}

func TestArchiveAPI_GetArchiveStatus_Success(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/archive/empty", nil)
	w := httptest.NewRecorder()
	api.CreateEmptyArchive(w, req)

	var resp createEmptyArchiveResp
	json.Unmarshal(w.Body.Bytes(), &resp)

	req = httptest.NewRequest(http.MethodGet, "/archive/status?archive_id="+resp.ID, nil)
	w = httptest.NewRecorder()

	api.GetArchiveStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var statusResp getArchiveStatusResp
	err := json.Unmarshal(w.Body.Bytes(), &statusResp)
	require.NoError(t, err)

	assert.Equal(t, resp.ID, statusResp.ID)
	assert.Equal(t, "empty", statusResp.Status)
}

func TestArchiveAPI_GetArchiveStatus_MissingArchiveID(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/archive/status", nil)
	w := httptest.NewRecorder()

	api.GetArchiveStatus(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "отсутствует archive_id")
}

func TestArchiveAPI_GetArchiveStatus_NotFound(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/archive/status?archive_id=nonexistent", nil)
	w := httptest.NewRecorder()

	api.GetArchiveStatus(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Архив не найден")
}

func TestArchiveAPI_DownloadArchive_MissingArchiveID(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/download", nil)
	w := httptest.NewRecorder()

	api.DownloadArchive(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "отсутствует archive_id")
}

func TestArchiveAPI_DownloadArchive_NotFound(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/download?archive_id=nonexistent", nil)
	w := httptest.NewRecorder()

	api.DownloadArchive(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Архив не найден")
}

func TestArchiveAPI_DownloadArchive_NotReady(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/archive/empty", nil)
	w := httptest.NewRecorder()
	api.CreateEmptyArchive(w, req)

	var resp createEmptyArchiveResp
	json.Unmarshal(w.Body.Bytes(), &resp)

	req = httptest.NewRequest(http.MethodGet, "/download?archive_id="+resp.ID, nil)
	w = httptest.NewRecorder()

	api.DownloadArchive(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Архив недоступен для скачивания")
}

func TestArchiveAPI_CreateArchive_PartialSuccess(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := createArchiveReq{
		URLs: []string{
			"https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf",
			"invalid-url",
			"https://httpbin.org/status/404",
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.CreateArchive(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp createArchiveResp
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "ready", resp.Status)
	assert.Len(t, resp.Files, 1)
	assert.Len(t, resp.Errors, 2)
	assert.NotEmpty(t, resp.ArchiveURL)
}

func TestArchiveAPI_CreateArchive_ServiceError(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := createArchiveReq{
		URLs: []string{"invalid-url", "another-invalid-url"},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.CreateArchive(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestArchiveAPI_CreateEmptyArchive_ServiceError(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/archive/empty", nil)
		w := httptest.NewRecorder()
		api.CreateEmptyArchive(w, req)
	}

	req := httptest.NewRequest(http.MethodPost, "/archive/empty", nil)
	w := httptest.NewRecorder()

	api.CreateEmptyArchive(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "сервер занят")
}

func TestArchiveAPI_DownloadArchive_Ready(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	reqBody := createArchiveReq{
		URLs: []string{
			"https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf",
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/archive", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateArchive(w, req)

	var resp createArchiveResp
	json.Unmarshal(w.Body.Bytes(), &resp)

	req = httptest.NewRequest(http.MethodGet, "/download?archive_id="+resp.ID, nil)
	w = httptest.NewRecorder()

	api.DownloadArchive(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
}

func TestArchiveAPI_AddFile_ServiceError(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/archive/empty", nil)
	w := httptest.NewRecorder()
	api.CreateEmptyArchive(w, req)

	var resp createEmptyArchiveResp
	json.Unmarshal(w.Body.Bytes(), &resp)

	reqBody := addFileReq{
		URL: "invalid-url",
	}

	body, _ := json.Marshal(reqBody)
	req = httptest.NewRequest(http.MethodPost, "/archive/add-file?archive_id="+resp.ID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	api.AddFile(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var addResp addFileResp
	err := json.Unmarshal(w.Body.Bytes(), &addResp)
	require.NoError(t, err)

	assert.False(t, addResp.Success)
	assert.Contains(t, addResp.Message, "некорректный URL файла")
}

func TestArchiveAPI_GetArchiveStatus_ServiceError(t *testing.T) {
	api, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/archive/status?archive_id=nonexistent", nil)
	w := httptest.NewRecorder()

	api.GetArchiveStatus(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Архив не найден")
}
