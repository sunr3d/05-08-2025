package middleware

import (
	"net/http"
	"runtime/debug"
	"strings"

	"go.uber.org/zap"
)

func ReqLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Info("Входящий HTTP запрос",
				zap.String("method", r.Method),
				zap.String("url", r.URL.Path),
			)
			next.ServeHTTP(w, r)
		})
	}
}

func JSONValidator() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && requiresJSON(r.URL.Path) {
				ct := r.Header.Get("Content-Type")
				base := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
				if base != "application/json" {
					http.Error(w, "Неверный Content-Type, ожидается application/json", http.StatusUnsupportedMediaType)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requiresJSON(path string) bool {
	endpoints := map[string]bool{
		"/archive":          true,
		"/archive/add-file": true,
	}

	return endpoints[path]
}

func Recovery(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("Паника в обработчике запроса",
						zap.Any("error", err),
						zap.String("stack", string(debug.Stack())),
						zap.String("url", r.URL.Path),
						zap.String("method", r.Method),
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error": "Внутренняя ошибка сервера"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
