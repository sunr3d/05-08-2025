package services

import (
	"context"

	"github.com/sunr3d/05-08-2025/models"
)

//go:generate go run github.com/vektra/mockery/v2@v2.53.2 --name=ArchiveService --output=../../../mocks
type ArchiveService interface {
	CreateArchive(ctx context.Context, urls []string) (*models.Archive, error)

	CreateEmptyArchive(ctx context.Context) (*models.Archive, error)
	AddFile(ctx context.Context, archiveID, fileURL string) error
	GetArchiveStatus(ctx context.Context, archiveID string) (*models.Archive, error)
}
