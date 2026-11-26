package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tokenRequest is the JSON body for POST /api/v1/token.
type tokenRequest struct {
	Room     string `json:"room"`
	Identity string `json:"identity"`
	Name     string `json:"name,omitempty"`
	JoinPin  string `json:"join_pin,omitempty"`
	Mode     string `json:"mode,omitempty"`      // 已无效: 房间模式由 create 接口建房时定死, 进房只以存储为准
	UserAddr string `json:"user_addr,omitempty"` // 进房者 0x 地址; 房间绑定了 user_addr 时必填且必须一致
}

// tokenResponse is the JSON response for POST /api/v1/token.
type tokenResponse struct {
	Code    int    `json:"code"`
	Room    string `json:"room,omitempty"`
	Token   string `json:"token,omitempty"`
	WsURL   string `json:"ws_url,omitempty"`
	IsOwner bool   `json:"is_owner"`
	Mode    string `json:"mode,omitempty"` // 房间固定模式: video/voice (服务端权威值)
	Error   string `json:"error,omitempty"`
}

// sanitizeMode normalizes a room mode: only "voice" accepted, else "video".
func sanitizeMode(raw string) string {
	m := strings.ToLower(strings.TrimSpace(raw))
	if m != roomModeVoice {
		return roomModeVideo
	}
	return m
}

// livekitTokenHandler handles POST /api/v1/token — validates room/pin and issues a LiveKit access token.
func livekitTokenHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "method not allowed"})
			return
		}

		var req tokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "invalid json"})
			return
		}

		room := sanitizeRoom(req.Room)
		if len(room) < 4 || len(room) > 48 {
			json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "room must be 4-48 characters"})
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = "guest"
		}

		identity := strings.TrimSpace(req.Identity)
		if identity == "" {
			identity = fmt.Sprintf("u_%d", time.Now().UnixNano())
		}

		// PIN validation: 严格模式 —— 房间必须先经 POST /api/v1/room/create 创建,
		// 不存在的房间直接拒绝, 不接受链接带 pin 隐式建房 (防乱拼链接占用房间号)
		joinPin := strings.TrimSpace(req.JoinPin)
		exists := roomExists(room)

		if !exists {
			json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "room not found"})
			return
		} else {
			// Existing room: verify PIN (带尝试限流, 防暴力猜口令)
			if roomPinLocked(room) {
				json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "too many pin attempts, try again later"})
				return
			}
			if !roomPinMatches(room, joinPin) {
				roomPinFailIncr(room)
				json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "wrong pin"})
				return
			}
			roomPinFailReset(room)
		}

		// 用户地址校验: 建房时绑定了 user_addr 的房间, 进房者必须是绑定用户 (user_addr)
		// 或房主本人 (owner) —— 两者任一匹配即放行, 房主创建房间有权进房
		if bindAddr, ok := roomGetUserAddr(room); ok && bindAddr != "" {
			provided, _ := isValidAddr(req.UserAddr)
			ownerAddr, _ := roomGetOwner(room)
			if provided == "" || (provided != bindAddr && provided != ownerAddr) {
				json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "user address not allowed"})
				return
			}
		}

		// 房间模式以存储值为准(create 接口建房时定死), 请求中的 mode 已无效
		mode, _ := roomGetMode(room)

		// Build LiveKit access token
		token, err := buildAccessToken(cfg.LiveKit, room, identity, name, tokenTTL, mode)
		if err != nil {
			json.NewEncoder(w).Encode(tokenResponse{Code: 1, Error: "token generation failed: " + err.Error()})
			return
		}

		wsURL := wsURLForClient(r, cfg.LiveKit.WSURL)

		json.NewEncoder(w).Encode(tokenResponse{
			Code:    0,
			Room:    room,
			Token:   token,
			WsURL:   wsURL,
			IsOwner: false, // 严格模式下房间只能由 create 接口创建, 进房者永不为房主
			Mode:    mode,
		})
	}
}

// buildAccessToken creates a HS256-signed JWT for LiveKit room access.
func buildAccessToken(lk LiveKitConfig, room, identity, name string, ttl time.Duration, mode string) (string, error) {
	video := map[string]any{
		"room":     room,
		"roomJoin": true,
		// canPublishSources 只是收窄 canPublish 的范围, 不能替代它
		"canPublish":   true,
		"canSubscribe": true,
	}
	maxParticipants := 5
	if mode == roomModeVoice {
		// 语音房间: 服务端强制只允许发布麦克风, 摄像头想开都开不了
		video["canPublishSources"] = []string{"microphone"}
		maxParticipants = 10
	}
	// 人数上限随 token 下发 (LiveKit ClaimGrants.RoomConfig), 由首个进房者创建房间时生效。
	// 这样无需修改 LiveKit 服务器全局的 room.max_participants。
	roomConfig := map[string]any{
		"max_participants": maxParticipants,
	}
	return signAccessToken(lk, identity, name, video, roomConfig, ttl)
}

// signAccessToken signs a LiveKit-compatible JWT with HS256.
func signAccessToken(lk LiveKitConfig, sub, name string, video, roomConfig map[string]any, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	now := time.Now().Unix()
	payload := map[string]any{
		"iss":        strings.TrimSpace(lk.APIKey),
		"sub":        sub,
		"nbf":        now - 5,
		"iat":        now,
		"exp":        now + int64(ttl.Seconds()),
		"video":      video,
		"roomConfig": roomConfig,
	}
	if strings.TrimSpace(name) != "" {
		payload["name"] = name
	}

	header := map[string]any{
		"alg": "HS256",
		"typ": "JWT",
	}

	h, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	p, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding
	head := enc.EncodeToString(h)
	body := enc.EncodeToString(p)
	unsigned := head + "." + body

	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(lk.APISecret)))
	if _, err := mac.Write([]byte(unsigned)); err != nil {
		return "", err
	}
	signature := enc.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

// wsURLForClient rewrites loopback ws_url to use the requesting client's host,
// so mobile clients don't try to connect to 127.0.0.1.
func wsURLForClient(r *http.Request, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return ""
	}
	u, err := url.Parse(configured)
	if err != nil || u.Host == "" {
		return configured
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		port = ""
	}
	if !isLoopback(host) {
		return configured
	}
	reqHost := requestHostForClient(r)
	if reqHost == "" || isLoopback(reqHost) {
		return configured
	}
	var hostport string
	if port != "" {
		hostport = net.JoinHostPort(reqHost, port)
	} else if u.Scheme == "wss" {
		hostport = net.JoinHostPort(reqHost, "443")
	} else {
		hostport = net.JoinHostPort(reqHost, "7880")
	}
	out := *u
	out.Host = hostport
	return out.String()
}

func isLoopback(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	switch host {
	case "localhost", "127.0.0.1", "::1", "0:0:0:0:0:0:0:1":
		return true
	default:
		return strings.HasPrefix(host, "127.")
	}
}

func requestHostForClient(r *http.Request) string {
	if r == nil {
		return ""
	}
	candidate := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if candidate != "" {
		if i := strings.IndexByte(candidate, ','); i >= 0 {
			candidate = strings.TrimSpace(candidate[:i])
		}
		candidate = strings.TrimPrefix(strings.TrimPrefix(candidate, "http://"), "https://")
		if i := strings.IndexByte(candidate, '/'); i >= 0 {
			candidate = candidate[:i]
		}
	}
	if candidate == "" {
		candidate = r.Host
	}
	host, _, err := net.SplitHostPort(candidate)
	if err != nil {
		return strings.TrimSpace(candidate)
	}
	return strings.TrimSpace(host)
}
