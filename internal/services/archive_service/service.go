package archive_service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/sunr3d/05-08-2025/internal/config"
	"github.com/sunr3d/05-08-2025/internal/interfaces/infra"
	"github.com/sunr3d/05-08-2025/internal/interfaces/services"
	"github.com/sunr3d/05-08-2025/models"
)

var _ services.ArchiveService = (*archiveService)(nil)

type archiveService struct {
	repo       infra.Database
	logger     *zap.Logger
	cfg        *config.Config
	httpClient *http.Client
}

func New(log *zap.Logger, cfg *config.Config, repo infra.Database) services.ArchiveService {
	return &archiveService{
		logger:     log,
		cfg:        cfg,
		repo:       repo,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

func (s *archiveService) CreateArchive(ctx context.Context, urls []string) (*models.Archive, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	limit, err := s.repo.CountArchivesInProcess(ctx)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить количество архивов в процессе сборки: %w", err)
	}

	if limit >= s.cfg.MaxArchivesInProcess {
		return nil, ErrServerBusy
	}

	if len(urls) > s.cfg.MaxFilesPerArchive {
		return nil, fmt.Errorf("%w: %v", ErrMaxFilesPerArchive, len(urls))
	}

	archiveID := uuid.New().String()
	archive := &models.Archive{
		ID:        archiveID,
		Status:    models.ArchiveStatusBuilding,
		Files:     make([]string, 0, len(urls)),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Errors:    make([]string, 0, len(urls)),
	}

	for _, url := range urls {
		if !s.isValidURL(url) {
			archive.Errors = append(archive.Errors, fmt.Sprintf("%s - %s", url, ErrInvalidFileURL.Error()))
			continue
		}

		fileReader, filename, err := s.downloadFile(ctx, url)
		if err != nil {
			archive.Errors = append(archive.Errors, fmt.Sprintf("%s - %s", url, err.Error()))
			continue
		}

		func() {
			defer fileReader.Close()
			if err := s.saveFile(ctx, archiveID, filename, fileReader); err != nil {
				archive.Errors = append(archive.Errors, fmt.Sprintf("%s - %s", url, err.Error()))
				return
			}
			archive.Files = append(archive.Files, filename)
		}()
	}

	if len(archive.Files) > 0 {
		if err := s.buildZip(ctx, archiveID, archive.Files); err != nil {
			archive.Status = models.ArchiveStatusFailed
			archive.Errors = append(archive.Errors, fmt.Sprintf("%s: %s", ErrArchiveBuild.Error(), err.Error()))
		} else {
			archive.Status = models.ArchiveStatusReady
			if err := s.cleanupTemp(ctx, archiveID); err != nil {
				s.logger.Error("не удалось очистить временные файлы",
					zap.String("archive_id", archiveID),
					zap.Error(err),
				)
			}
		}
	} else {
		archive.Status = models.ArchiveStatusFailed
	}

	err = s.repo.SaveArchive(ctx, archive)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrArchiveSave, err)
	}

	if len(archive.Files) > 0 {
		s.logger.Info("архив собран",
			zap.String("archive_id", archive.ID),
			zap.String("status", string(archive.Status)),
			zap.Int("total_urls", len(urls)),
			zap.Int("successful_files", len(archive.Files)),
			zap.Int("errors", len(archive.Errors)),
		)
	} else {
		s.logger.Info("архив не создан, нет доступных файлов",
			zap.String("archive_id", archive.ID),
			zap.String("status", string(archive.Status)),
			zap.Int("total_urls", len(urls)),
			zap.Int("successful_files", len(archive.Files)),
			zap.Int("errors", len(archive.Errors)),
		)
	}
	return archive, nil
}

func (s *archiveService) CreateEmptyArchive(ctx context.Context) (*models.Archive, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	limit, err := s.repo.CountArchivesInProcess(ctx)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить количество архивов в процессе сборки: %w", err)
	}

	if limit >= s.cfg.MaxArchivesInProcess {
		return nil, ErrServerBusy
	}

	archiveID := uuid.New().String()
	archive := &models.Archive{
		ID:        archiveID,
		Status:    models.ArchiveStatusEmpty,
		Files:     make([]string, 0, s.cfg.MaxFilesPerArchive),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Errors:    make([]string, 0, s.cfg.MaxFilesPerArchive),
	}

	err = s.repo.SaveArchive(ctx, archive)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrArchiveSave, err)
	}

	s.logger.Info("пустой архив создан", zap.String("archive_id", archive.ID))
	return archive, nil
}

func (s *archiveService) AddFile(ctx context.Context, archiveID, fileURL string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	archive, err := s.repo.GetArchive(ctx, archiveID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrArchiveGet, err)
	}

	if archive.Status == models.ArchiveStatusReady {
		return ErrArchiveReady
	}
	if archive.Status == models.ArchiveStatusFailed {
		return ErrArchiveFailed
	}

	if len(archive.Files) >= s.cfg.MaxFilesPerArchive {
		return ErrArchiveFull
	}

	if !s.isValidURL(fileURL) {
		return ErrInvalidFileURL
	}

	fileReader, filename, err := s.downloadFile(ctx, fileURL)
	if err != nil {
		s.logger.Error("не удалось загрузить файл",
			zap.String("archive_id", archiveID),
			zap.String("file_url", fileURL),
			zap.Error(err),
		)
		return fmt.Errorf("%w: %v", ErrFileDownloadFailed, err)
	}
	defer fileReader.Close()

	if err := s.saveFile(ctx, archiveID, filename, fileReader); err != nil {
		s.logger.Error("не удалось сохранить файл",
			zap.String("archive_id", archiveID),
			zap.String("file_url", fileURL),
			zap.Error(err),
		)
		return fmt.Errorf("%w: %v", ErrFileCopyFailed, err)
	}

	archive.Files = append(archive.Files, filename)
	archive.UpdatedAt = time.Now()
	if archive.Status == models.ArchiveStatusEmpty {
		archive.Status = models.ArchiveStatusBuilding
	}
	s.logger.Info("файл добавлен в архив",
		zap.String("archive_id", archiveID),
		zap.String("filename", filename),
		zap.String("archive_status", string(archive.Status)),
	)

	if len(archive.Files) == s.cfg.MaxFilesPerArchive {
		if err := s.buildZip(ctx, archiveID, archive.Files); err != nil {
			archive.Status = models.ArchiveStatusFailed
			archive.Errors = append(archive.Errors, fmt.Sprintf("%s: %s", ErrArchiveBuild.Error(), err.Error()))
		} else {
			archive.Status = models.ArchiveStatusReady
			if err := s.cleanupTemp(ctx, archiveID); err != nil {
				s.logger.Error("не удалось очистить временные файлы",
					zap.String("archive_id", archiveID),
					zap.Error(err),
				)
			}
			s.logger.Info("архив собран", zap.String("archive_id", archiveID))
		}
	}

	err = s.repo.SaveArchive(ctx, archive)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrArchiveSave, err)
	}

	return nil
}

func (s *archiveService) GetArchive(ctx context.Context, archiveID string) (*models.Archive, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	archive, err := s.repo.GetArchive(ctx, archiveID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrArchiveGet, err)
	}

	s.logger.Info("статус архива получен",
		zap.String("archive_id", archiveID),
		zap.String("status", string(archive.Status)),
	)

	return archive, nil
}

func (s *archiveService) isValidURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func (s *archiveService) isValidExt(contentType string) bool {
	contentType = strings.Split(contentType, ";")[0]
	contentType = strings.TrimSpace(contentType)

	for _, allowed := range s.cfg.AllowedExtensions {
		if contentType == allowed {
			return true
		}
	}
	return false
}

func (s *archiveService) downloadFile(ctx context.Context, url string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidFileURL, err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrFileDownloadFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%w: HTTP status %d", ErrFileDownloadFailed, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !s.isValidExt(contentType) {
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedFile, contentType)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrFileDownloadFailed, err)
	}

	filename := path.Base(url)
	return io.NopCloser(bytes.NewReader(data)), filename, nil
}

func (s *archiveService) saveFile(ctx context.Context, archiveID, filename string, fileReader io.ReadCloser) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	dir := filepath.Join(s.cfg.TempDir, archiveID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("%w: %v", ErrMkdirFailed, err)
	}

	filePath := filepath.Join(dir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFileCreateFailed, err)
	}
	defer file.Close()

	_, err = io.Copy(file, fileReader)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFileCopyFailed, err)
	}

	return nil
}

func (s *archiveService) buildZip(ctx context.Context, archiveID string, files []string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	tempDir := filepath.Join(s.cfg.TempDir, archiveID)
	zipPath := filepath.Join(s.cfg.ArchivesDir, archiveID+".zip")

	if err := os.MkdirAll(s.cfg.ArchivesDir, 0755); err != nil {
		return fmt.Errorf("%w: %v", ErrMkdirFailed, err)
	}

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFileCreateFailed, err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, filename := range files {
		filePath := filepath.Join(tempDir, filename)
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrFileOpenFailed, err)
		}
		defer file.Close()

		w, err := zipWriter.Create(filename)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrFileCreateFailed, err)
		}

		if _, err = io.Copy(w, file); err != nil {
			return fmt.Errorf("%w: %v", ErrFileCopyFailed, err)
		}
	}

	return nil
}

func (s *archiveService) cleanupTemp(ctx context.Context, archiveID string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	tempDir := filepath.Join(s.cfg.TempDir, archiveID)
	if err := os.RemoveAll(tempDir); err != nil {
		return fmt.Errorf("%w: %v", ErrRemoveFailed, err)
	}

	return nil
}
