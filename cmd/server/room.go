package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// 服务端生成房间号字符集: 去掉易混淆的 0/1/o/i/l (链接直达, 用户无需手输)
const (
	roomNameAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
	roomNameLen      = 6
	roomPinLen       = 4
)

// PIN 尝试限流: 同一房间连续错 maxPinAttempts 次后锁定 pinLockTTL
const (
	pinFailKeyPrefix = "h5room:pinfail:"
	maxPinAttempts   = 5
	pinLockTTL       = 10 * time.Minute
)

// Redis key scheme: room data is stored as a HASH `h5room:room:<room>` with
// fields { pin_hash, mode }. Room names are pre-sanitized (lowercase
// alphanumeric + underscore, ≤48 chars) before any room op, so no escaping
// is needed.
const roomKeyPrefix = "h5room:room:"

// room hash field names
const (
	roomFieldPinHash  = "pin_hash"
	roomFieldMode     = "mode"
	roomFieldOwner    = "owner"     // 房主 0x 地址 (App 建房时设置)
	roomFieldUserAddr = "user_addr" // 目标用户 0x 地址 (App 建房时绑定, 进房时校验一致)
)

// 房主列表: Hash { 0x地址: 最近建房 unix 时间 } — 建房带 owner 时更新
const ownersKey = "h5room:owners"

// room modes
const (
	roomModeVideo = "video"
	roomModeVoice = "voice"
)

func roomKey(room string) string { return roomKeyPrefix + room }

// pinHash returns the SHA256 hex digest of a PIN string.
func pinHash(pin string) string {
	h := sha256.Sum256([]byte(pin))
	return hex.EncodeToString(h[:])
}

// roomExists checks whether a room has been registered.
func roomExists(room string) bool {
	ctx, cancel := redisCtx()
	defer cancel()
	n, err := rdb.Exists(ctx, roomKey(room)).Result()
	if err != nil {
		log.Printf("警告: 查询房间 %s 失败: %v", room, err)
		return false
	}
	return n > 0
}

// isValidAddr 校验 0x 钱包地址: 0x 前缀 + 40 位十六进制 (42 字符), 返回规范化小写形式。
func isValidAddr(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != 42 || !strings.HasPrefix(s, "0x") {
		return "", false
	}
	if _, err := hex.DecodeString(s[2:]); err != nil {
		return "", false
	}
	return s, true
}

// roomCreate records a new room with its PIN hash, mode, and optional
// owner 0x address / bound user 0x address (empty strings skip the field).
// Atomic create via HSETNX on pin_hash.
func roomCreate(room, pin, mode, owner, userAddr string) error {
	if len(pin) < 1 || len(pin) > 32 {
		return fmt.Errorf("pin must be 1-32 characters")
	}
	if len(room) < 4 {
		return fmt.Errorf("room name too short")
	}
	if mode != roomModeVoice {
		mode = roomModeVideo
	}
	if owner != "" {
		owner, _ = isValidAddr(owner)
		if owner == "" {
			return fmt.Errorf("invalid owner address")
		}
	}
	if userAddr != "" {
		userAddr, _ = isValidAddr(userAddr)
		if userAddr == "" {
			return fmt.Errorf("invalid user_addr")
		}
	}

	ctx, cancel := redisCtx()
	defer cancel()
	ok, err := rdb.HSetNX(ctx, roomKey(room), roomFieldPinHash, pinHash(pin)).Result()
	if err != nil {
		return fmt.Errorf("redis unavailable: %w", err)
	}
	if !ok {
		return fmt.Errorf("room already exists")
	}
	if err := rdb.HSet(ctx, roomKey(room), roomFieldMode, mode).Err(); err != nil {
		return fmt.Errorf("redis unavailable: %w", err)
	}
	if owner != "" {
		if err := rdb.HSet(ctx, roomKey(room), roomFieldOwner, owner).Err(); err != nil {
			return fmt.Errorf("redis unavailable: %w", err)
		}
		// 房主列表: 记录最近建房时间
		if err := rdb.HSet(ctx, ownersKey, owner, time.Now().Unix()).Err(); err != nil {
			log.Printf("警告: 更新房主列表失败: %v", err)
		}
	}
	if userAddr != "" {
		// 统一存规范化小写, 进房校验时同样规范化后比对
		if err := rdb.HSet(ctx, roomKey(room), roomFieldUserAddr, userAddr).Err(); err != nil {
			return fmt.Errorf("redis unavailable: %w", err)
		}
	}
	if roomPinTTL > 0 {
		if err := rdb.Expire(ctx, roomKey(room), roomPinTTL).Err(); err != nil {
			log.Printf("警告: 设置房间 %s 过期时间失败: %v", room, err)
		}
	}
	return nil
}

// roomPinMatches checks a PIN against the stored hash for a room.
func roomPinMatches(room, pin string) bool {
	ctx, cancel := redisCtx()
	defer cancel()
	val, err := rdb.HGet(ctx, roomKey(room), roomFieldPinHash).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		log.Printf("警告: 校验房间 %s 口令失败: %v", room, err)
		return false
	}
	return subtle.ConstantTimeCompare([]byte(val), []byte(pinHash(pin))) == 1
}

// roomDelete removes a room record.
func roomDelete(room string) {
	ctx, cancel := redisCtx()
	defer cancel()
	if err := rdb.Del(ctx, roomKey(room)).Err(); err != nil {
		log.Printf("警告: 删除房间 %s 失败: %v", room, err)
	}
}

// roomGetPinStatus returns whether a PIN is set for the room.
func roomGetPinStatus(room string) (pinSet bool, found bool) {
	ctx, cancel := redisCtx()
	defer cancel()
	n, err := rdb.Exists(ctx, roomKey(room)).Result()
	if err != nil {
		log.Printf("警告: 查询房间 %s 状态失败: %v", room, err)
		return false, false
	}
	if n == 0 {
		return false, false
	}
	return true, true
}

// roomGetMode returns the fixed mode of a room ("video"/"voice").
// Missing field or unknown value defaults to "video".
func roomGetMode(room string) (mode string, found bool) {
	ctx, cancel := redisCtx()
	defer cancel()
	val, err := rdb.HGet(ctx, roomKey(room), roomFieldMode).Result()
	if err == redis.Nil {
		return roomModeVideo, false
	}
	if err != nil {
		log.Printf("警告: 查询房间 %s 模式失败: %v", room, err)
		return roomModeVideo, false
	}
	if val != roomModeVoice {
		return roomModeVideo, true
	}
	return roomModeVoice, true
}

// randomToken 从 alphabet 中随机取 n 个字符 (crypto/rand 均匀分布).
// crypto/rand 失败极罕见, 回退纳秒时间戳保证不重样。
func randomToken(alphabet string, n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = alphabet[(time.Now().UnixNano()+int64(i))%int64(len(alphabet))]
			continue
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}

// roomCreateRandom 生成随机房间号 + 4 位数字 PIN 并注册房间 (供 /api/v1/room/create 使用).
// owner/userIP 为空时跳过绑定。撞号时自动重试 (HSETNX 原子抢注), 返回 (room, pin, mode)。
// 所有房间带 h5_ 前缀, 用于与其他服务的 LiveKit 房间隔离;
// 语音房间再加 voice_ 段, 人数上限由 token 的 roomConfig.max_participants 决定 (语音 10 / 视频 5).
func roomCreateRandom(mode, owner, userIP string) (string, string, string, error) {
	mode = sanitizeMode(mode)
	for attempt := 0; attempt < 10; attempt++ {
		room := randomToken(roomNameAlphabet, roomNameLen)
		if mode == roomModeVoice {
			room = "h5_voice_" + room
		} else {
			room = "h5_" + room
		}
		pin := randomToken("0123456789", roomPinLen)
		err := roomCreate(room, pin, mode, owner, userIP)
		if err == nil {
			return room, pin, mode, nil
		}
		if !strings.Contains(err.Error(), "already exists") {
			return "", "", "", err
		}
		// 撞号: 重试 (概率极低)
	}
	return "", "", "", fmt.Errorf("failed to generate a unique room")
}

// roomGetOwner 返回房间房主 0x 地址 (建房时 App 提供), 未设置时 ok=false。
func roomGetOwner(room string) (string, bool) {
	ctx, cancel := redisCtx()
	defer cancel()
	val, err := rdb.HGet(ctx, roomKey(room), roomFieldOwner).Result()
	if err == redis.Nil {
		return "", false
	}
	if err != nil {
		log.Printf("警告: 查询房间 %s 房主地址失败: %v", room, err)
		return "", false
	}
	return val, true
}

// roomGetUserAddr 返回房间绑定的目标用户 0x 地址 (建房时 App 提供), 未绑定时 ok=false。
func roomGetUserAddr(room string) (string, bool) {
	ctx, cancel := redisCtx()
	defer cancel()
	val, err := rdb.HGet(ctx, roomKey(room), roomFieldUserAddr).Result()
	if err == redis.Nil {
		return "", false
	}
	if err != nil {
		log.Printf("警告: 查询房间 %s 绑定地址失败: %v", room, err)
		return "", false
	}
	return val, true
}

// roomOwnerEntry 房主列表条目
type roomOwnerEntry struct {
	Addr          string `json:"addr"`
	LastCreatedAt int64  `json:"last_created_at"`
}

// roomOwnerList 返回全部房主 (0x 地址 + 最近建房时间), 按最近建房时间倒序。
func roomOwnerList() []roomOwnerEntry {
	ctx, cancel := redisCtx()
	defer cancel()
	m, err := rdb.HGetAll(ctx, ownersKey).Result()
	if err != nil {
		log.Printf("警告: 查询房主列表失败: %v", err)
		return nil
	}
	list := make([]roomOwnerEntry, 0, len(m))
	for addr, tsStr := range m {
		ts, _ := strconv.ParseInt(tsStr, 10, 64)
		list = append(list, roomOwnerEntry{Addr: addr, LastCreatedAt: ts})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].LastCreatedAt > list[j].LastCreatedAt })
	return list
}

// ---- PIN 尝试限流: 连续错 maxPinAttempts 次, 锁定 pinLockTTL ----

func roomPinFailCount(room string) int {
	ctx, cancel := redisCtx()
	defer cancel()
	n, err := rdb.Get(ctx, pinFailKeyPrefix+room).Int()
	if err == redis.Nil {
		return 0
	}
	if err != nil {
		log.Printf("警告: 查询房间 %s 锁定状态失败: %v", room, err)
		return 0
	}
	return n
}

// roomPinFailIncr 记录一次口令错误, 返回累计错误次数 (首次记录时设置 TTL)
func roomPinFailIncr(room string) int {
	ctx, cancel := redisCtx()
	defer cancel()
	key := pinFailKeyPrefix + room
	n, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		log.Printf("警告: 记录房间 %s 口令错误失败: %v", room, err)
		return 0
	}
	if n == 1 {
		if err := rdb.Expire(ctx, key, pinLockTTL).Err(); err != nil {
			log.Printf("警告: 设置房间 %s 锁定 TTL 失败: %v", room, err)
		}
	}
	return int(n)
}

// roomPinFailReset 口令校验成功后清零错误计数
func roomPinFailReset(room string) {
	ctx, cancel := redisCtx()
	defer cancel()
	if err := rdb.Del(ctx, pinFailKeyPrefix+room).Err(); err != nil {
		log.Printf("警告: 清零房间 %s 口令计数失败: %v", room, err)
	}
}

// roomPinLocked 房间是否处于锁定状态 (拒绝一切加入尝试)
func roomPinLocked(room string) bool {
	return roomPinFailCount(room) >= maxPinAttempts
}
