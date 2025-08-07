package archive_service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/sunr3d/05-08-2025/internal/config"
	"github.com/sunr3d/05-08-2025/internal/infra/inmem"
	"github.com/sunr3d/05-08-2025/models"

	"github.com/stretchr/testify/mock"
	mocks "github.com/sunr3d/05-08-2025/mocks"
)

const (
	testPDFURL  = "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf"
	testJPEGURL = "https://httpbin.org/image/jpeg"
	testPNG     = "https://httpbin.org/image/png"
	invalidURL  = "invalid-url"
	notFoundURL = "https://httpbin.org/status/404"
)

func setupTestService(t *testing.T) (*archiveService, func()) {
	logger := zaptest.NewLogger(t)

	tempDir := filepath.Join(os.TempDir(), "archive_service_test_"+time.Now().Format("20060102150405"))
	archivesDir := filepath.Join(tempDir, "archives")
	tempFilesDir := filepath.Join(tempDir, "temp")

	cfg := &config.Config{
		HTTPTimeout:          30 * time.Second,
		AllowedExtensions:    []string{"application/pdf", "image/jpeg", "image/jpg"},
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		ArchiveTTL:           1 * time.Hour,
		ArchivesDir:          archivesDir,
		TempDir:              tempFilesDir,
	}

	repo := inmem.New(logger, cfg.ArchiveTTL)
	service := New(logger, cfg, repo).(*archiveService)

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return service, cleanup
}

func TestArchiveService_CreateArchive_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	urls := []string{testPDFURL}

	archive, err := service.CreateArchive(ctx, urls)

	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusReady, archive.Status)
	assert.Len(t, archive.Files, 1)
	assert.Empty(t, archive.Errors)
	assert.NotEmpty(t, archive.ID)

	zipPath := filepath.Join(service.cfg.ArchivesDir, archive.ID+".zip")
	_, err = os.Stat(zipPath)
	assert.NoError(t, err)
}

func TestArchiveService_CreateArchive_PartialSuccess(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	urls := []string{testPDFURL, invalidURL}

	archive, err := service.CreateArchive(ctx, urls)

	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusReady, archive.Status)
	assert.Len(t, archive.Files, 1)
	assert.Len(t, archive.Errors, 1)

	zipPath := filepath.Join(service.cfg.ArchivesDir, archive.ID+".zip")
	_, err = os.Stat(zipPath)
	assert.NoError(t, err)
}

func TestArchiveService_CreateArchive_AllFailed(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	urls := []string{invalidURL, notFoundURL, testPNG}

	archive, err := service.CreateArchive(ctx, urls)

	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.Empty(t, archive.Files)
	assert.Len(t, archive.Errors, 3)
}

func TestArchiveService_CreateArchive_TooManyFiles(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	urls := []string{testPDFURL, testPDFURL, testPDFURL, testPDFURL}

	_, err := service.CreateArchive(ctx, urls)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "превышен лимит файлов в архиве")
}

func TestArchiveService_CreateArchive_ServerBusy(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		archive := &models.Archive{
			ID:        "test-busy-" + string(rune('a'+i)),
			Status:    models.ArchiveStatusBuilding,
			Files:     []string{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Errors:    []string{},
		}
		err := service.repo.SaveArchive(ctx, archive)
		require.NoError(t, err)
	}

	urls := []string{testPDFURL}
	_, err := service.CreateArchive(ctx, urls)

	assert.Error(t, err)
	assert.Equal(t, ErrServerBusy, err)
}

func TestArchiveService_CreateEmptyArchive_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive, err := service.CreateEmptyArchive(ctx)

	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusEmpty, archive.Status)
	assert.Empty(t, archive.Files)
	assert.Empty(t, archive.Errors)
	assert.NotEmpty(t, archive.ID)
}

func TestArchiveService_AddFile_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testPDFURL)
	require.NoError(t, err)

	updatedArchive, err := service.GetArchive(ctx, archive.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusBuilding, updatedArchive.Status)
	assert.Len(t, updatedArchive.Files, 1)
}

func TestArchiveService_AddFile_ArchiveBecomesReady(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testPDFURL)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testJPEGURL)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testPDFURL)
	require.NoError(t, err)

	updatedArchive, err := service.GetArchive(ctx, archive.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusReady, updatedArchive.Status)
	assert.Len(t, updatedArchive.Files, 3)
}

func TestArchiveService_AddFile_ArchiveNotFound(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	err := service.AddFile(ctx, "nonexistent-id", testPDFURL)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось получить архив")
}

func TestArchiveService_AddFile_ArchiveReady(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive := &models.Archive{
		ID:        "test-ready",
		Status:    models.ArchiveStatusReady,
		Files:     []string{"file1.pdf"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Errors:    []string{},
	}
	err := service.repo.SaveArchive(ctx, archive)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testPDFURL)

	assert.Error(t, err)
	assert.Equal(t, ErrArchiveReady, err)
}

func TestArchiveService_AddFile_ArchiveFailed(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive := &models.Archive{
		ID:        "test-failed",
		Status:    models.ArchiveStatusFailed,
		Files:     []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Errors:    []string{"some error"},
	}
	err := service.repo.SaveArchive(ctx, archive)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testPDFURL)

	assert.Error(t, err)
	assert.Equal(t, ErrArchiveFailed, err)
}

func TestArchiveService_AddFile_ArchiveFull(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive := &models.Archive{
		ID:        "test-full",
		Status:    models.ArchiveStatusBuilding,
		Files:     []string{"file1.pdf", "file2.jpg", "file3.pdf"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Errors:    []string{},
	}
	err := service.repo.SaveArchive(ctx, archive)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testPDFURL)

	assert.Error(t, err)
	assert.Equal(t, ErrArchiveFull, err)
}

func TestArchiveService_AddFile_InvalidURL(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, invalidURL)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidFileURL, err)
}

func TestArchiveService_GetArchive_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	originalArchive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	archive, err := service.GetArchive(ctx, originalArchive.ID)

	require.NoError(t, err)
	assert.Equal(t, originalArchive.ID, archive.ID)
	assert.Equal(t, originalArchive.Status, archive.Status)
}

func TestArchiveService_GetArchive_NotFound(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	_, err := service.GetArchive(ctx, "nonexistent-id")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось получить архив")
}

func TestArchiveService_isValidURL(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com/file.pdf", true},
		{"http://example.com/file.jpg", true},
		{"ftp://example.com/file.pdf", false},
		{"invalid-url", false},
		{"", false},
	}

	for _, test := range tests {
		result := service.isValidURL(test.url)
		assert.Equal(t, test.expected, result, "URL: %s", test.url)
	}
}

func TestArchiveService_isValidExt(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	tests := []struct {
		contentType string
		expected    bool
	}{
		{"application/pdf", true},
		{"image/jpeg", true},
		{"image/jpg", true},
		{"image/png", false},
		{"text/plain", false},
		{"application/pdf; charset=utf-8", true},
		{"", false},
	}

	for _, test := range tests {
		result := service.isValidExt(test.contentType)
		assert.Equal(t, test.expected, result, "Content-Type: %s", test.contentType)
	}
}

func TestArchiveService_downloadFile_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	reader, filename, err := service.downloadFile(ctx, testPDFURL)

	require.NoError(t, err)
	assert.NotNil(t, reader)
	assert.NotEmpty(t, filename)

	reader.Close()
}

func TestArchiveService_downloadFile_InvalidURL(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	_, _, err := service.downloadFile(ctx, invalidURL)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось загрузить файл")
}

func TestArchiveService_downloadFile_NotFound(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	_, _, err := service.downloadFile(ctx, notFoundURL)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось загрузить файл")
}

func TestArchiveService_saveFile_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-save"
	filename := "test.txt"

	testData := "test file content"
	reader := io.NopCloser(bytes.NewReader([]byte(testData)))

	err := service.saveFile(ctx, archiveID, filename, reader)

	require.NoError(t, err)

	filePath := filepath.Join(service.cfg.TempDir, archiveID, filename)
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, testData, string(content))
}

func TestArchiveService_buildZip_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-zip"
	files := []string{"file1.txt", "file2.txt"}

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	for _, filename := range files {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte("test content"), 0644)
		require.NoError(t, err)
	}

	err = service.buildZip(ctx, archiveID, files)
	require.NoError(t, err)

	zipPath := filepath.Join(service.cfg.ArchivesDir, archiveID+".zip")
	_, err = os.Stat(zipPath)
	assert.NoError(t, err)
}

func TestArchiveService_cleanupTemp_Success(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-cleanup"

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	testFile1 := filepath.Join(tempDir, "test1.txt")
	testFile2 := filepath.Join(tempDir, "test2.pdf")
	subDir := filepath.Join(tempDir, "subdir")
	testFile3 := filepath.Join(subDir, "test3.jpg")

	err = os.WriteFile(testFile1, []byte("test1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(testFile2, []byte("test2"), 0644)
	require.NoError(t, err)
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(testFile3, []byte("test3"), 0644)
	require.NoError(t, err)

	_, err = os.Stat(testFile1)
	require.NoError(t, err)
	_, err = os.Stat(testFile2)
	require.NoError(t, err)
	_, err = os.Stat(testFile3)
	require.NoError(t, err)

	err = service.cleanupTemp(ctx, archiveID)
	require.NoError(t, err)

	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestArchiveService_cleanupTemp_DirectoryNotExists(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-cleanup-nonexistent"

	err := service.cleanupTemp(ctx, archiveID)
	require.NoError(t, err)
}

func TestArchiveService_cleanupTemp_EmptyDirectory(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-cleanup-empty"

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	err = service.cleanupTemp(ctx, archiveID)
	require.NoError(t, err)

	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestArchiveService_cleanupTemp_WithSpecialCharacters(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-cleanup-special"

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	specialFile := filepath.Join(tempDir, "test file with spaces.txt")
	err = os.WriteFile(specialFile, []byte("test"), 0644)
	require.NoError(t, err)

	cyrillicFile := filepath.Join(tempDir, "тест.txt")
	err = os.WriteFile(cyrillicFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = service.cleanupTemp(ctx, archiveID)
	require.NoError(t, err)

	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestArchiveService_cleanupTemp_WithLargeFiles(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-cleanup-large"

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	largeFile := filepath.Join(tempDir, "large.txt")
	largeData := make([]byte, 1024*1024) // 1MB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	err = os.WriteFile(largeFile, largeData, 0644)
	require.NoError(t, err)

	err = service.cleanupTemp(ctx, archiveID)
	require.NoError(t, err)

	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestArchiveService_cleanupTemp_ContextCanceled(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	archiveID := "test-cleanup-context"

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	err = service.cleanupTemp(ctx, archiveID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "отмена контекста")

	_, err = os.Stat(tempDir)
	assert.NoError(t, err)
}

func TestArchiveService_cleanupTemp_WithReadOnlyFiles(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-cleanup-readonly"

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	readonlyFile := filepath.Join(tempDir, "readonly.txt")
	err = os.WriteFile(readonlyFile, []byte("test"), 0444)
	require.NoError(t, err)

	err = service.cleanupTemp(ctx, archiveID)
	require.NoError(t, err)

	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestArchiveService_cleanupTemp_WithHiddenFiles(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "test-cleanup-hidden"

	tempDir := filepath.Join(service.cfg.TempDir, archiveID)
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)

	hiddenFile := filepath.Join(tempDir, ".hidden.txt")
	err = os.WriteFile(hiddenFile, []byte("test"), 0644)
	require.NoError(t, err)

	hiddenDir := filepath.Join(tempDir, ".hidden_dir")
	err = os.MkdirAll(hiddenDir, 0755)
	require.NoError(t, err)

	hiddenFileInDir := filepath.Join(hiddenDir, "test.txt")
	err = os.WriteFile(hiddenFileInDir, []byte("test"), 0644)
	require.NoError(t, err)

	err = service.cleanupTemp(ctx, archiveID)
	require.NoError(t, err)

	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestArchiveService_CreateArchive_ContextCanceled(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	urls := []string{testPDFURL}
	_, err := service.CreateArchive(ctx, urls)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "отмена контекста")
}

func TestArchiveService_AddFile_ContextCanceled(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	archive, err := service.CreateEmptyArchive(context.Background())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = service.AddFile(ctx, archive.ID, testPDFURL)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "отмена контекста")
}

func TestArchiveService_GetArchive_ContextCanceled(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.GetArchive(ctx, "some-id")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "отмена контекста")
}

func TestArchiveService_CreateArchive_WithJPEG(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	urls := []string{testJPEGURL}

	archive, err := service.CreateArchive(ctx, urls)

	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusReady, archive.Status)
	assert.Len(t, archive.Files, 1)
	assert.Empty(t, archive.Errors)
	assert.NotEmpty(t, archive.ID)

	zipPath := filepath.Join(service.cfg.ArchivesDir, archive.ID+".zip")
	_, err = os.Stat(zipPath)
	assert.NoError(t, err)
}

func TestArchiveService_AddFile_WithJPEG(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	err = service.AddFile(ctx, archive.ID, testJPEGURL)
	require.NoError(t, err)

	updatedArchive, err := service.GetArchive(ctx, archive.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusBuilding, updatedArchive.Status)
	assert.Len(t, updatedArchive.Files, 1)
}

func TestArchiveService_CreateArchive_BuildZipFail(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := "application/pdf"
		if strings.HasSuffix(r.URL.Path, ".jpg") || strings.HasSuffix(r.URL.Path, ".jpeg") {
			ct = "image/jpeg"
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("DATA"))
	}))
	defer ts.Close()

	badPath := filepath.Join(service.cfg.ArchivesDir, "archives_as_file2")
	require.NoError(t, os.MkdirAll(filepath.Dir(badPath), 0755))
	require.NoError(t, os.WriteFile(badPath, []byte("x"), 0644))
	service.cfg.ArchivesDir = badPath

	ctx := context.Background()
	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	require.NoError(t, service.AddFile(ctx, archive.ID, ts.URL+"/a.pdf"))
	require.NoError(t, service.AddFile(ctx, archive.ID, ts.URL+"/b.jpeg"))
	require.NoError(t, service.AddFile(ctx, archive.ID, ts.URL+"/c.pdf"))

	updated, err := service.GetArchive(ctx, archive.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, updated.Status)
	require.NotEmpty(t, updated.Errors)
}

func TestArchiveService_AddFile_BuildZipFail(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := "application/pdf"
		if strings.HasSuffix(r.URL.Path, ".jpg") || strings.HasSuffix(r.URL.Path, ".jpeg") {
			ct = "image/jpeg"
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("DATA"))
	}))
	defer ts.Close()

	badPath := filepath.Join(service.cfg.ArchivesDir, "archives_as_file2")
	require.NoError(t, os.MkdirAll(filepath.Dir(badPath), 0755))
	require.NoError(t, os.WriteFile(badPath, []byte("x"), 0644))
	service.cfg.ArchivesDir = badPath

	ctx := context.Background()
	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	require.NoError(t, service.AddFile(ctx, archive.ID, ts.URL+"/a.pdf"))
	require.NoError(t, service.AddFile(ctx, archive.ID, ts.URL+"/b.jpeg"))
	require.NoError(t, service.AddFile(ctx, archive.ID, ts.URL+"/c.pdf"))

	updated, err := service.GetArchive(ctx, archive.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, updated.Status)
	require.NotEmpty(t, updated.Errors)
}

func TestArchiveService_downloadFile_UnsupportedContentType(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PNGDATA"))
	}))
	defer ts.Close()

	_, _, err := service.downloadFile(ctx, ts.URL+"/x.png")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "неподдерживаемый файл")
}

func TestArchiveService_CreateArchive_SaveError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tempDir := filepath.Join(os.TempDir(), "svc_save_err_"+time.Now().Format("20060102150405"))
	archivesDir := filepath.Join(tempDir, "archives")
	tempFilesDir := filepath.Join(tempDir, "temp")
	require.NoError(t, os.MkdirAll(archivesDir, 0755))
	require.NoError(t, os.MkdirAll(tempFilesDir, 0755))

	cfg := &config.Config{
		HTTPTimeout:          30 * time.Second,
		AllowedExtensions:    []string{"application/pdf", "image/jpeg", "image/jpg"},
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		ArchiveTTL:           1 * time.Hour,
		ArchivesDir:          archivesDir,
		TempDir:              tempFilesDir,
	}

	mockRepo := new(mocks.Database)
	mockRepo.On("CountArchivesInProcess", mock.Anything).Return(0, nil).Once()
	mockRepo.On("SaveArchive", mock.Anything, mock.Anything).Return(assert.AnError).Once()

	svc := New(logger, cfg, mockRepo).(*archiveService)

	_, err := svc.CreateArchive(context.Background(), []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось сохранить архив")

	os.RemoveAll(tempDir)
}

func TestArchiveService_AddFile_SaveError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tempDir := filepath.Join(os.TempDir(), "svc_addfile_save_err_"+time.Now().Format("20060102150405"))
	archivesDir := filepath.Join(tempDir, "archives")
	tempFilesDir := filepath.Join(tempDir, "temp")
	require.NoError(t, os.MkdirAll(archivesDir, 0755))
	require.NoError(t, os.MkdirAll(tempFilesDir, 0755))

	cfg := &config.Config{
		HTTPTimeout:          30 * time.Second,
		AllowedExtensions:    []string{"application/pdf", "image/jpeg", "image/jpg"},
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		ArchiveTTL:           1 * time.Hour,
		ArchivesDir:          archivesDir,
		TempDir:              tempFilesDir,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PDFDATA"))
	}))
	defer ts.Close()

	mockRepo := new(mocks.Database)
	arch := &models.Archive{
		ID:        "arch-1",
		Status:    models.ArchiveStatusEmpty,
		Files:     []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Errors:    []string{},
	}
	mockRepo.On("GetArchive", mock.Anything, arch.ID).Return(arch, nil).Once()
	mockRepo.On("SaveArchive", mock.Anything, mock.AnythingOfType("*models.Archive")).Return(assert.AnError).Once()

	svc := New(logger, cfg, mockRepo).(*archiveService)

	err := svc.AddFile(context.Background(), arch.ID, ts.URL+"/test.pdf")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось сохранить архив")

	os.RemoveAll(tempDir)
}

func TestArchiveService_CreateArchive_CountInProcessError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tempDir := filepath.Join(os.TempDir(), "svc_cnt_err_"+time.Now().Format("20060102150405"))
	archivesDir := filepath.Join(tempDir, "archives")
	tempFilesDir := filepath.Join(tempDir, "temp")
	require.NoError(t, os.MkdirAll(archivesDir, 0755))
	require.NoError(t, os.MkdirAll(tempFilesDir, 0755))

	cfg := &config.Config{
		HTTPTimeout:          30 * time.Second,
		AllowedExtensions:    []string{"application/pdf", "image/jpeg", "image/jpg"},
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		ArchiveTTL:           1 * time.Hour,
		ArchivesDir:          archivesDir,
		TempDir:              tempFilesDir,
	}

	mockRepo := new(mocks.Database)
	mockRepo.On("CountArchivesInProcess", mock.Anything).Return(0, assert.AnError).Once()

	svc := New(logger, cfg, mockRepo).(*archiveService)

	_, err := svc.CreateArchive(context.Background(), []string{testPDFURL})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось получить количество архивов")

	os.RemoveAll(tempDir)
}

func TestArchiveService_CreateEmptyArchive_SaveError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tempDir := filepath.Join(os.TempDir(), "svc_empty_save_err_"+time.Now().Format("20060102150405"))
	archivesDir := filepath.Join(tempDir, "archives")
	tempFilesDir := filepath.Join(tempDir, "temp")
	require.NoError(t, os.MkdirAll(archivesDir, 0755))
	require.NoError(t, os.MkdirAll(tempFilesDir, 0755))

	cfg := &config.Config{
		HTTPTimeout:          30 * time.Second,
		AllowedExtensions:    []string{"application/pdf", "image/jpeg", "image/jpg"},
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		ArchiveTTL:           1 * time.Hour,
		ArchivesDir:          archivesDir,
		TempDir:              tempFilesDir,
	}

	mockRepo := new(mocks.Database)
	mockRepo.On("CountArchivesInProcess", mock.Anything).Return(0, nil).Once()
	mockRepo.On("SaveArchive", mock.Anything, mock.AnythingOfType("*models.Archive")).Return(assert.AnError).Once()

	svc := New(logger, cfg, mockRepo).(*archiveService)

	_, err := svc.CreateEmptyArchive(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось сохранить архив")

	os.RemoveAll(tempDir)
}

func TestArchiveService_AddFile_DownloadError(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	err = service.AddFile(ctx, archive.ID, ts.URL+"/file.pdf")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось загрузить файл")
}

func TestArchiveService_AddFile_SaveFileMkdirError(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	badTemp := filepath.Join(service.cfg.TempDir, "bad_temp_file")
	require.NoError(t, os.MkdirAll(filepath.Dir(badTemp), 0755))
	require.NoError(t, os.WriteFile(badTemp, []byte("x"), 0644))
	service.cfg.TempDir = badTemp

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PDFDATA"))
	}))
	defer ts.Close()

	err = service.AddFile(ctx, archive.ID, ts.URL+"/file.pdf")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось скопировать файл")
}

func TestArchiveService_AddFile_SaveFileCreateError(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archive, err := service.CreateEmptyArchive(ctx)
	require.NoError(t, err)

	tempDir := filepath.Join(service.cfg.TempDir, archive.ID)
	require.NoError(t, os.MkdirAll(tempDir, 0755))
	badName := "bad.pdf"
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, badName), 0755))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PDFDATA"))
	}))
	defer ts.Close()

	err = service.AddFile(ctx, archive.ID, ts.URL+"/"+badName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось создать файл")
}

func TestArchiveService_buildZip_FileOpenError(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	archiveID := "zip_open_err"
	files := []string{"missing1.txt", "missing2.txt"}

	err := service.buildZip(ctx, archiveID, files)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось открыть файл")
}
