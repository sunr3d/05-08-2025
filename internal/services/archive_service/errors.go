package archive_service

import "errors"

var (
	ErrContextDone = errors.New("отмена контекста")

	ErrServerBusy = errors.New("сервер занят, максимальное количество архивов в процессе достигнуто")

	ErrMaxFilesPerArchive = errors.New("превышен лимит файлов в архиве")

	ErrArchiveFull = errors.New("архив заполнен")
	ErrArchiveSave = errors.New("не удалось сохранить архив")
	ErrArchiveGet  = errors.New("не удалось получить архив")
	ErrArchiveBuild = errors.New("не удалось создать архив")

	ErrFileNotFound       = errors.New("файл не найден")
	ErrUnsupportedFile    = errors.New("неподдерживаемый файл")
	ErrFileDownloadFailed = errors.New("не удалось загрузить файл")
	ErrInvalidFileURL     = errors.New("некорректный URL файла")

	ErrMkdirFailed      = errors.New("не удалось создать директорию")
	ErrFileCreateFailed = errors.New("не удалось создать файл")
	ErrFileOpenFailed   = errors.New("не удалось открыть файл")
	ErrFileCopyFailed   = errors.New("не удалось скопировать файл")
	ErrRemoveFailed     = errors.New("не удалось удалить файл/директорию")
)
