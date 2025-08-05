package config

type Config struct {
	HTTPPort string `envconfig:"HTTP_PORT" default:"8080"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	AllowedExtensions []string `envconfig:"ALLOWED_EXTENSIONS" default:".pdf,.jpeg"`
	MaxTasks int `envconfig:"MAX_TASKS" default:"3"`
	MaxFilesPerTask int `envconfig:"MAX_FILES_PER_TASK" default:"3"`
}