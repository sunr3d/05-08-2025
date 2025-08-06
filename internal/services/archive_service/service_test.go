package archive_service

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/sunr3d/05-08-2025/internal/config"
	"github.com/sunr3d/05-08-2025/internal/infra/inmem"
	"github.com/sunr3d/05-08-2025/models"
)

func TestMain(m *testing.M) {
	os.MkdirAll("./testdata/tmp", 0755)
	os.MkdirAll("./testdata/archives", 0755)
	code := m.Run()
	os.RemoveAll("./testdata/")
	os.Exit(code)
}

func newTestService() *archiveService {
	cfg := &config.Config{
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		AllowedExtensions:    []string{"application/pdf", "image/jpeg", "image/jpg"},
		TempDir:              "./testdata/tmp",
		ArchivesDir:          "./testdata/archives",
		HTTPTimeout:          time.Second * 10,
	}
	logger := zap.NewNop()
	repo := inmem.New(logger, time.Hour)
	return &archiveService{
		repo:       repo,
		logger:     logger,
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

func TestNew(t *testing.T) {
	cfg := &config.Config{
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		AllowedExtensions:    []string{"application/pdf"},
		TempDir:              "./testdata/tmp",
		ArchivesDir:          "./testdata/archives",
		HTTPTimeout:          time.Second,
	}
	logger := zap.NewNop()
	repo := inmem.New(logger, time.Hour)

	service := New(logger, cfg, repo)
	assert.NotNil(t, service)
}

func TestCreateArchive_Success(t *testing.T) {
	svc := newTestService()

	urls := []string{
		"https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf",
		"https://httpbin.org/image/jpeg",
	}

	archive, err := svc.CreateArchive(context.Background(), urls)
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusReady, archive.Status)
	assert.Len(t, archive.Files, 2)
	assert.Empty(t, archive.Errors)
}

func TestCreateArchive_InvalidURL(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{"bad-url"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.Empty(t, archive.Files)
	assert.NotEmpty(t, archive.Errors)
}

func TestCreateArchive_TooManyFiles(t *testing.T) {
	svc := newTestService()

	_, err := svc.CreateArchive(context.Background(), []string{"a", "b", "c", "d"})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMaxFilesPerArchive)
}

func TestCreateArchive_PartialSuccess(t *testing.T) {
	svc := newTestService()

	urls := []string{
		"https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf",
		"bad-url",
	}

	archive, err := svc.CreateArchive(context.Background(), urls)
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusReady, archive.Status)
	assert.Len(t, archive.Files, 1)
	assert.Len(t, archive.Errors, 1)
}

func TestCreateEmptyArchive_AndAddFile(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateEmptyArchive(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusEmpty, archive.Status)

	err = svc.AddFile(context.Background(), archive.ID, "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf")
	assert.NoError(t, err)

	archive2, err := svc.GetArchiveStatus(context.Background(), archive.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusBuilding, archive2.Status)
	assert.Len(t, archive2.Files, 1)
}

func TestAddFile_ArchiveFull(t *testing.T) {
	svc := newTestService()
	archive, _ := svc.CreateEmptyArchive(context.Background())

	for i := 0; i < 3; i++ {
		err := svc.AddFile(context.Background(), archive.ID, "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf")
		assert.NoError(t, err)
	}

	err := svc.AddFile(context.Background(), archive.ID, "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf")
	assert.ErrorIs(t, err, ErrArchiveFull)
}

func TestCreateArchive_TooManyArchives(t *testing.T) {
	svc := newTestService()

	for i := 0; i < 3; i++ {
		_, err := svc.CreateEmptyArchive(context.Background())
		assert.NoError(t, err)
	}

	// Пытаемся создать 4-й архив
	_, err := svc.CreateArchive(context.Background(), []string{"https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf"})
	assert.ErrorIs(t, err, ErrServerBusy)
}

func TestGetArchiveStatus_NotFound(t *testing.T) {
	svc := newTestService()

	_, err := svc.GetArchiveStatus(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestCreateArchive_UnsupportedFileType(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{"https://httpbin.org/robots.txt"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.Empty(t, archive.Files)
	assert.NotEmpty(t, archive.Errors)
}

func TestCreateArchive_EmptyURLs(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.Empty(t, archive.Files)
	assert.Empty(t, archive.Errors)
}

func TestCreateArchive_ContextCancelled(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.CreateArchive(ctx, []string{"https://example.com/file.pdf"})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrContextDone)
}

func TestCreateEmptyArchive_ContextCancelled(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.CreateEmptyArchive(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrContextDone)
}

func TestAddFile_ContextCancelled(t *testing.T) {
	svc := newTestService()
	archive, _ := svc.CreateEmptyArchive(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.AddFile(ctx, archive.ID, "https://example.com/file.pdf")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrContextDone)
}

func TestGetArchiveStatus_ContextCancelled(t *testing.T) {
	svc := newTestService()
	archive, _ := svc.CreateEmptyArchive(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.GetArchiveStatus(ctx, archive.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrContextDone)
}

func TestAddFile_InvalidURL(t *testing.T) {
	svc := newTestService()
	archive, _ := svc.CreateEmptyArchive(context.Background())

	err := svc.AddFile(context.Background(), archive.ID, "bad-url")
	assert.ErrorIs(t, err, ErrInvalidFileURL)
}

func TestAddFile_ArchiveNotFound(t *testing.T) {
	svc := newTestService()

	err := svc.AddFile(context.Background(), "nonexistent", "https://example.com/file.pdf")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrArchiveGet)
}

func TestCreateArchive_NetworkTimeout(t *testing.T) {

	cfg := &config.Config{
		MaxArchivesInProcess: 3,
		MaxFilesPerArchive:   3,
		AllowedExtensions:    []string{"application/pdf"},
		TempDir:              "./testdata/tmp",
		ArchivesDir:          "./testdata/archives",
		HTTPTimeout:          time.Millisecond, // Очень короткий таймаут
	}
	logger := zap.NewNop()
	repo := inmem.New(logger, time.Hour)
	svc := &archiveService{
		repo:       repo,
		logger:     logger,
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}

	archive, err := svc.CreateArchive(context.Background(), []string{"https://httpbin.org/delay/5"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.NotEmpty(t, archive.Errors)
}

func TestCreateArchive_HTTPError(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{"https://nonexistent-domain-12345.com/file.pdf"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.NotEmpty(t, archive.Errors)
}

func TestCreateArchive_HTTP404(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{"https://httpbin.org/status/404"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.NotEmpty(t, archive.Errors)
}

func TestCreateArchive_HTTP500(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{"https://httpbin.org/status/500"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.NotEmpty(t, archive.Errors)
}

func TestCreateArchive_InvalidContentType(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{"https://httpbin.org/headers"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.NotEmpty(t, archive.Errors)
}

func TestCreateArchive_AllFilesFailed(t *testing.T) {
	svc := newTestService()

	archive, err := svc.CreateArchive(context.Background(), []string{"bad-url-1", "bad-url-2", "bad-url-3"})
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusFailed, archive.Status)
	assert.Empty(t, archive.Files)
	assert.Len(t, archive.Errors, 3)
}

func TestAddFile_ArchiveFullAfterAdding(t *testing.T) {
	svc := newTestService()
	archive, _ := svc.CreateEmptyArchive(context.Background())

	for i := 0; i < 3; i++ {
		err := svc.AddFile(context.Background(), archive.ID, "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf")
		assert.NoError(t, err)
	}

	archive2, err := svc.GetArchiveStatus(context.Background(), archive.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.ArchiveStatusReady, archive2.Status)
	assert.Len(t, archive2.Files, 3)
}

func TestCreateArchive_ServerBusyEmptyArchive(t *testing.T) {
	svc := newTestService()

	for i := 0; i < 3; i++ {
		_, err := svc.CreateEmptyArchive(context.Background())
		assert.NoError(t, err)
	}

	_, err := svc.CreateEmptyArchive(context.Background())
	assert.ErrorIs(t, err, ErrServerBusy)
}
