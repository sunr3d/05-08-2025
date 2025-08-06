package config

import "time"

type Config struct {
	HTTPPort string `envconfig:"HTTP_PORT" default:"8080"`
	HTTPTimeout time.Duration `envconfig:"HTTP_TIMEOUT" default:"30s"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	AllowedExtensions []string `envconfig:"ALLOWED_EXTENSIONS" default:"application/pdf,image/jpeg,image/jpg"`
	MaxArchivesInProcess int `envconfig:"MAX_ARCHIVES_IN_PROCESS" default:"3"`
	MaxFilesPerArchive int `envconfig:"MAX_FILES_PER_ARCHIVE" default:"3"`
	ArchiveTTL time.Duration `envconfig:"ARCHIVE_TTL" default:"1h"`
	ArchivesDir string `envconfig:"ARCHIVES_DIR" default:"./data/archives"`
	TempDir string `envconfig:"TEMP_DIR" default:"./data/temp"`
}