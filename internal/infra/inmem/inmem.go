package inmem

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sunr3d/05-08-2025/internal/interfaces/infra"
	"github.com/sunr3d/05-08-2025/models"
)

var _ infra.Database = (*inmemDB)(nil)

type inmemDB struct {
	logger *zap.Logger
	db     map[string]*models.Archive
	mu     sync.RWMutex
	ttl    time.Duration
}

func New(log *zap.Logger, ttl time.Duration) infra.Database {
	return &inmemDB{
		logger: log,
		db:     make(map[string]*models.Archive),
		ttl:    ttl,
	}
}

func (db *inmemDB) SaveArchive(ctx context.Context, archive *models.Archive) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	if archive == nil {
		return ErrArchiveNil
	}

	if archive.ID == "" {
		return ErrArchiveIDEmpty
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.db[archive.ID] = archive
	db.logger.Info("архив сохранен", zap.String("archive_id", archive.ID))

	return nil
}

func (db *inmemDB) GetArchive(ctx context.Context, id string) (*models.Archive, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	if id == "" {
		return nil, ErrArchiveIDEmpty
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	archive, exists := db.db[id]
	if !exists {
		return nil, ErrArchiveNotFound
	}

	return archive, nil
}

func (db *inmemDB) CountArchivesInProcess(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
		return 0, fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	count := 0
	now := time.Now()

	for id, archive := range db.db {
		if archive.Status == models.ArchiveStatusBuilding ||
			archive.Status == models.ArchiveStatusEmpty {
			if now.Sub(archive.UpdatedAt) > db.ttl {
				delete(db.db, id)
				db.logger.Info("архив удален по TTL", zap.String("archive_id", id))
				continue
			}
			count++
		}
	}

	return count, nil
}

func (db *inmemDB) DeleteArchive(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", ErrContextDone, ctx.Err())
	default:
	}

	if id == "" {
		return ErrArchiveIDEmpty
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.db[id]; !exists {
		return ErrArchiveNotFound
	}

	delete(db.db, id)
	db.logger.Info("архив удален", zap.String("archive_id", id))

	return nil
}
