package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Printf("warning: %v — using defaults", err)
	}

	initRedis(cfg.Redis)

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/v1/token", livekitTokenHandler(cfg))
	mux.HandleFunc("/api/v1/room/create", roomCreateHandler())
	mux.HandleFunc("/api/v1/room/owners", roomOwnersHandler())
	mux.HandleFunc("/api/v1/room/pin-status", roomPinStatusHandler())

	// Static file server for Vue production build
	fs := http.FileServer(http.Dir(cfg.Server.WebDir))
	mux.Handle("/", noCacheIndex(fs))

	scheme := "http"
	if cfg.Server.TLSCert != "" || cfg.Server.TLSKey != "" {
		scheme = "https"
	}

	listenAddr := cfg.Server.Listen
	if strings.HasPrefix(listenAddr, ":") {
		listenAddr = "127.0.0.1" + listenAddr
	}
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("  h5_room API 启动")
	log.Printf("  监听: %s://%s", scheme, listenAddr)
	log.Printf("  LiveKit API: %s", cfg.LiveKit.APIBase)
	log.Printf("  Redis: %s (db %d)", cfg.Redis.Addr, cfg.Redis.DB)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("  POST /api/v1/token              获取接入令牌")
	log.Printf("  POST /api/v1/room/create        创建房间 (服务端生成随机房间号+PIN)")
	log.Printf("  GET  /api/v1/room/owners        房主列表 (0x 地址)")
	log.Printf("  GET  /api/v1/room/pin-status    查询房间口令状态")
	log.Printf("  GET  /                          静态文件 (web/dist/)")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	handler := apiLogMiddleware(mux)
	if scheme == "https" {
		if cfg.Server.TLSCert == "" || cfg.Server.TLSKey == "" {
			log.Fatal("tls_cert 和 tls_key 必须同时配置")
		}
		log.Printf("  TLS 证书: %s", cfg.Server.TLSCert)
		if err := http.ListenAndServeTLS(cfg.Server.Listen, cfg.Server.TLSCert, cfg.Server.TLSKey, handler); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := http.ListenAndServe(cfg.Server.Listen, handler); err != nil {
		log.Fatal(err)
	}
}

// noCacheIndex 让 index.html 每次重新验证: App WebView 缓存激进,
// 旧 index.html 会引用已被新构建删除的 hash 资源 → 页面黑屏或跑旧逻辑。
// 带 hash 的资源文件 (assets/*) 保持默认缓存即可。
func noCacheIndex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, "/index.html") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// roomCreateRequest is the JSON body for POST /api/v1/room/create.
type roomCreateRequest struct {
	Mode     string `json:"mode,omitempty"`      // 可选: "voice"; 缺省 video
	Owner    string `json:"owner,omitempty"`     // 房主 0x 地址 (可选; 提供后保存入房主列表)
	UserAddr string `json:"user_addr,omitempty"` // 目标用户 0x 地址 (可选; 绑定后进房时校验一致)
}

// roomCreateResponse is the JSON response for POST /api/v1/room/create.
type roomCreateResponse struct {
	Code     int    `json:"code"`
	Room     string `json:"room,omitempty"`
	Pin      string `json:"pin,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Owner    string `json:"owner,omitempty"`
	UserAddr string `json:"user_addr,omitempty"`
	Error    string `json:"error,omitempty"`
}

// roomCreateHandler handles POST /api/v1/room/create — App 端创建房间,
// 服务端生成随机房间号 + PIN, App 拿到后拼接入链接发给用户。
// owner 保存入房主列表; user_addr 绑定后该房间进房必须是绑定用户或房主本人。
func roomCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(roomCreateResponse{Code: 1, Error: "method not allowed"})
			return
		}
		var req roomCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(roomCreateResponse{Code: 1, Error: "invalid json"})
			return
		}
		room, pin, mode, err := roomCreateRandom(req.Mode, req.Owner, req.UserAddr)
		if err != nil {
			json.NewEncoder(w).Encode(roomCreateResponse{Code: 1, Error: err.Error()})
			return
		}
		json.NewEncoder(w).Encode(roomCreateResponse{Code: 0, Room: room, Pin: pin, Mode: mode, Owner: req.Owner, UserAddr: req.UserAddr})
	}
}

// roomOwnersResponse is the JSON response for GET /api/v1/room/owners.
type roomOwnersResponse struct {
	Code   int              `json:"code"`
	Owners []roomOwnerEntry `json:"owners,omitempty"`
	Error  string           `json:"error,omitempty"`
}

// roomOwnersHandler handles GET /api/v1/room/owners — 房主列表 (0x 地址 + 最近建房时间)。
func roomOwnersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(roomOwnersResponse{Code: 1, Error: "method not allowed"})
			return
		}
		json.NewEncoder(w).Encode(roomOwnersResponse{Code: 0, Owners: roomOwnerList()})
	}
}

// roomPinStatusHandler handles GET /api/v1/room/pin-status.
func roomPinStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"code":1,"error":"method not allowed"}`))
			return
		}
		room := sanitizeRoom(r.URL.Query().Get("room"))
		if len(room) < 4 {
			w.Write([]byte(`{"code":1,"error":"invalid room"}`))
			return
		}
		pinSet, found := roomGetPinStatus(room)
		if !found {
			w.Write([]byte(fmt.Sprintf(
				`{"code":0,"room":"%s","pin_set":false,"found":false}`,
				room,
			)))
			return
		}
		mode, _ := roomGetMode(room)
		w.Write([]byte(fmt.Sprintf(
			`{"code":0,"room":"%s","pin_set":%v,"found":true,"mode":"%s"}`,
			room, pinSet, mode,
		)))
	}
}
