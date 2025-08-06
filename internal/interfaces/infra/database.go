package infra

import (
	"context"

	"github.com/sunr3d/05-08-2025/models"
)

//go:generate go run github.com/vektra/mockery/v2@v2.53.2 --name=Database --output=../../../mocks
type Database interface {
	SaveArchive(ctx context.Context, archive *models.Archive) error
	GetArchive(ctx context.Context, id string) (*models.Archive, error)
	CountArchivesInProcess(ctx context.Context) (int, error)
	DeleteArchive(ctx context.Context, id string) error
}
