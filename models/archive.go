package models

import "time"

type ArchiveStatus string

const (
	ArchiveStatusEmpty ArchiveStatus = "empty"
	ArchiveStatusBuilding ArchiveStatus = "building"
	ArchiveStatusReady ArchiveStatus = "ready"
	ArchiveStatusFailed ArchiveStatus = "failed"
)

type Archive struct {
	ID string `json:"id"`
	Status ArchiveStatus `json:"status"`
	Files []string	 `json:"files"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Errors []string `json:"errors,omitempty"`
}