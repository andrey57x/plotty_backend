package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/redis"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
)

type ctxKey string

const UserIDKey ctxKey = "userID"

func WithUserID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, UserIDKey, id)
}

func GetUserID(ctx context.Context) (uint64, bool) {
	value := ctx.Value(UserIDKey)
	if value == nil {
		return 0, false
	}
	id, ok := value.(uint64)
	return id, ok
}

var allowedOrigins = map[string]bool{
	"https://plotty-stories.duckdns.org": true,
	"http://localhost:3000":              true,
	"http://localhost:8080":              true,
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		baseLogger := logger.FromContext(r.Context())
		reqLogger := baseLogger.With().Str("request_id", requestID).Logger()
		ctx := logger.ToContext(r.Context(), reqLogger)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func realIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	return ip
}

const maxBodyLogSize = 1024

func AccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log := logger.FromContext(r.Context())

		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		wInt := &responseWriterInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		defer func() {
			duration := time.Since(start)
			status := wInt.statusCode

			var logEvent *zerolog.Event
			msg := ""

			switch {
			case status >= 500:
				logEvent = log.Error()
				msg = "Server error"
			case status >= 400:
				logEvent = log.Info()
				msg = "Client error"
			default:
				logEvent = log.Info()
				msg = "Request processed successfully"
			}

			if len(bodyBytes) > 0 && len(bodyBytes) < maxBodyLogSize {
				logEvent = logEvent.Bytes("body", bodyBytes)
			}

			logEvent.
				Str("method", r.Method).
				Str("remote_addr", r.RemoteAddr).
				Str("url", r.URL.Path).
				Dur("work_time", duration).
				Int("status", status).
				Str("user_agent", r.UserAgent()).
				Str("host", r.Host).
				Str("real_ip", realIP(r)).
				Int64("content_length", r.ContentLength).
				Str("start_time", start.Format(time.RFC3339)).
				Str("duration_human", duration.String()).
				Int64("duration_ms", duration.Milliseconds()).
				Msg(msg)
		}()

		next.ServeHTTP(wInt, r)
	})
}

func AuthMiddleware(redisDB *redis.RedisDB) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := r.Cookie("session_id")
			if errors.Is(err, http.ErrNoCookie) {
				utilities.WriteError(w, http.StatusUnauthorized, "no session cookie")
				return
			}
			if err != nil {
				utilities.WriteError(w, http.StatusInternalServerError, "internal server error")
				return
			}

			userID, err := redisDB.GetUserIDBySession(r.Context(), session.Value)
			if err != nil {
				utilities.WriteError(w, http.StatusUnauthorized, "invalid session")
				return
			}

			ctx := WithUserID(r.Context(), userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
