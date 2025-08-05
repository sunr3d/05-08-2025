package infra

import (
	"context"

	"github.com/sunr3d/05-08-2025/models"
)

type Database interface {
	SaveArchive(ctx context.Context, archive *models.Archive) error
	GetArchive(ctx context.Context, id string) (*models.Archive, error)
	CountArchivesInProcess(ctx context.Context) (int, error)
	DeleteArchive(ctx context.Context, id string) error
}
