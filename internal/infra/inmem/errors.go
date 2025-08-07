package inmem

import "errors"

var (
	ErrArchiveNotFound = errors.New("архив не найден")
	ErrArchiveNil      = errors.New("архив не может быть nil")
	ErrArchiveIDEmpty  = errors.New("ID архива не может быть пустым")
	ErrContextDone     = errors.New("отмена контекста")
)
