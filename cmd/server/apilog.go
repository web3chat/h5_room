package main

// API 请求日志中间件 —— 记录每次 token 签发/房间查询请求。
// 请求/响应体截断 300 字符, 含客户端 IP 与耗时, 输出到 stdout (部署时进 journald/日志文件)。
// 注意: token 响应体含 LiveKit JWT (默认 2 小时有效), 会随日志记录。

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const maxLogBody = 300 // 请求/响应体日志截断长度

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLogBody {
		return s
	}
	return fmt.Sprintf("%s…(%d)", s[:maxLogBody], len(s))
}

func apiClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			xff = strings.TrimSpace(xff[:i])
		}
		return xff
	}
	return r.RemoteAddr
}

// skipAPILog 过滤静态资源, 只记录 API 调用。
func skipAPILog(r *http.Request) bool {
	p := r.URL.Path
	if p == "/" || strings.HasPrefix(p, "/assets/") {
		return true
	}
	if strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".js") ||
		strings.HasSuffix(p, ".css") || strings.HasSuffix(p, ".ico") ||
		strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".svg") {
		return true
	}
	return false
}

// logAPI 记录 API 请求, 含截断后的请求/响应体。
func logAPI(r *http.Request, status int, duration string, reqBody, respBody string) {
	if skipAPILog(r) {
		return
	}
	base := fmt.Sprintf("api: %s %s → %d (%s) remote=%s", r.Method, r.URL.Path, status, duration, apiClientIP(r))
	if reqBody != "" {
		base += " | req=" + truncate(reqBody)
	}
	if respBody != "" {
		base += " | resp=" + truncate(respBody)
	}
	log.Print(base)
}

// apiLogMiddleware wraps a handler with request/response body logging.
func apiLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 读取请求体（仅 POST JSON, 限制 1KB）
		var reqBody string
		if r.Method == http.MethodPost {
			if ct := r.Header.Get("Content-Type"); strings.Contains(ct, "json") {
				raw, _ := io.ReadAll(io.LimitReader(r.Body, 1024))
				r.Body.Close()
				reqBody = string(raw)
				r.Body = io.NopCloser(bytes.NewReader(raw))
			}
		}

		sw := &bodyWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)

		respBody := sw.body.String()
		logAPI(r, sw.status, time.Since(start).String(), reqBody, respBody)
	})
}

type bodyWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (bw *bodyWriter) WriteHeader(code int) {
	bw.status = code
	bw.ResponseWriter.WriteHeader(code)
}

func (bw *bodyWriter) Write(p []byte) (int, error) {
	if bw.body.Len() < maxLogBody*2 {
		bw.body.Write(p)
	}
	return bw.ResponseWriter.Write(p)
}
