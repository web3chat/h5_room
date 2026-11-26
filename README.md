# h5_room — H5 多人音视频房间

基于 [LiveKit](https://livekit.io/) SFU 的多人音视频 H5 页面。App 调用接口创建房间、生成带 PIN 的链接，用户打开链接即可在 App 内嵌 WebView 或外部浏览器进入语音/视频房间。

- **语音房**：最多 10 人，仅麦克风，实时说话高亮与音量条
- **视频房**：最多 5 人，自动开启摄像头 + 麦克风，3+2 网格布局
- **房间鉴权**：服务端随机房间号 + 4 位 PIN，错误 5 次锁定 10 分钟；可选绑定钱包地址
- **开箱即用**：Docker Compose 一键启动 Redis + LiveKit，Go 单二进制提供 API 与静态页面

## 架构

```
App (发现页/链接) → H5 (Vue 3 + LiveKit Client SDK) ↔ LiveKit Server (SFU/WebRTC)
                                        ↕
                                 Go 后端 (Token 签发 + 房间管理)
```

| 组件 | 作用 | 默认端口 |
|------|------|----------|
| Go 后端 | Token 签发、房间 PIN/mode 管理、静态文件服务、API 请求日志 | `:3000` |
| Vue H5 前端 | 加入房间 → 多人音视频 UI | Vite dev `:5173` |
| LiveKit Server | WebRTC SFU 信令与媒体中转 | `:7880` (WS), `:7881` (TCP), `:7882` (UDP) |
| Redis | 房间 PIN/模式/房主数据持久化 | `127.0.0.1:6379` |

## 目录结构

```
cmd/server/          Go 后端（配置、Redis、房间、Token、API 日志）
web/                 Vue 3 H5 前端
deploy/              systemd 与 nginx 部署示例
scripts/             自签名证书脚本（测试用）
docker-compose.yml   本地 Redis + LiveKit
livekit.yaml         LiveKit 配置
config.example.yaml  后端配置示例
```

## 快速开始

### 1. 环境要求

- Go 1.24+
- Node.js 20.19+ 或 22.12+（Vite 8 要求）
- Docker / Docker Compose

### 2. 启动基础设施（Redis + LiveKit Server）

```bash
cp .env.example .env       # 按需修改 REDIS_PASSWORD
docker compose up -d
```

- Redis：`127.0.0.1:6379`（仅本机访问），开启 AOF + 数据卷，容器重启不丢房间数据
- LiveKit Server：`7880` 信令 / `7881` TCP / `7882` UDP，配置见 `livekit.yaml`
- 本地开发时 Go 后端只监听 `127.0.0.1:3000`，无需对外放行；如需手机等外部设备
  直接连接 LiveKit，放行 `7880/tcp`、`7881/tcp`、`7882/udp`

### 3. 配置后端

```bash
cp config.example.yaml config.yaml
```

默认配置即可连接上面的本地基础设施：

```yaml
server:
  listen: ":3000"
  web_dir: "web/dist"

livekit:
  api_base: "http://127.0.0.1:7880"
  ws_url: "ws://127.0.0.1:7880"
  api_key: "devkey"
  api_secret: "secret"

redis:
  addr: "127.0.0.1:6379"
  password: "changeme"   # 与 .env 的 REDIS_PASSWORD 一致
  db: 0
  pin_ttl_seconds: 0     # 0 = 不过期; 86400 = 1 天
```

> `config.yaml` 与 `.env` 已在 `.gitignore` 中，不会被提交。生产环境务必更换 LiveKit
> `devkey/secret` 与 Redis 口令。

### 4. 启动 Go 后端

```bash
go run ./cmd/server -config config.yaml
```

### 5. 启动 Vue 前端（开发模式）

```bash
cd web
npm install
npm run dev
```

开发模式下 Vite 将 `/api` 代理到 Go 后端 `:3000`。

### 6. 建房并打开页面

严格模式：房间必须先经 `POST /api/v1/room/create` 创建，拼假链接进不去。

```bash
# 1. 建房（拿到 room + pin）
curl -X POST http://localhost:3000/api/v1/room/create \
  -H 'Content-Type: application/json' -d '{"mode":"voice"}'
# → {"code":0,"room":"h5_voice_abc234","pin":"8866","mode":"voice"}

# 2. 用返回的房间号 + PIN 打开页面
http://localhost:5173/?room=h5_voice_abc234&pin=8866&mode=voice
```

用多个浏览器窗口打开同一链接即可测试多人通话。

## 生产部署

### 构建与放置

```bash
# 本机交叉编译 Linux 静态二进制
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o server-linux ./cmd/server

# 前端生产构建
npm --prefix web ci
npm --prefix web run build

# 上传后（服务器）
sudo install -o root -g root -m 0755 server-linux /data/h5room/server
sudo rm -rf /data/h5room/web/dist
sudo cp -r web/dist /data/h5room/web/dist
```

服务器目录建议：

```
/data/h5room/server            # Linux amd64 静态二进制
/data/h5room/web/dist/         # Vue 生产构建
/data/h5room/config.yaml       # 0600, 含真实密钥，不提交仓库
```

### systemd

参考 [`deploy/h5room.service`](deploy/h5room.service)，按实际路径与用户调整：

```bash
sudo cp deploy/h5room.service /etc/systemd/system/h5room.service
sudo systemctl daemon-reload
sudo systemctl enable --now h5room
```

### nginx + HTTPS

WebRTC 的 `getUserMedia` 只在 HTTPS 安全上下文可用。推荐由 nginx 在 443 统一终结 TLS，
页面与 WSS 使用同一个 `host:port`：

- `/rtc` → `127.0.0.1:7880`（LiveKit WebSocket 信令，需透传 `Upgrade` / `Connection`）
- `/` → `127.0.0.1:3000`（H5 页面 + Go API）
- 透传 `X-Forwarded-Host`，后端会把 token 里的 loopback `ws_url` 改写为客户端可访问的域名

完整示例见 [`deploy/nginx-h5room.conf.example`](deploy/nginx-h5room.conf.example)：

```bash
# 1. DNS: 把域名 A/AAAA 记录指向服务器公网 IP
# 2. 申请证书（先只启用 80 端口 server 块）
sudo certbot certonly --nginx -d room.example.com
# 3. 启用完整配置
sudo cp deploy/nginx-h5room.conf.example /etc/nginx/sites-available/h5room
sudo vi /etc/nginx/sites-available/h5room   # 替换所有 room.example.com 占位
sudo ln -s /etc/nginx/sites-available/h5room /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

生产 `config.yaml` 关键项：

```yaml
server:
  listen: "127.0.0.1:3000"
  web_dir: "/data/h5room/web/dist"
livekit:
  api_base: "http://127.0.0.1:7880"
  ws_url: "wss://127.0.0.1:443"   # 后端按请求 Host 改写为 wss://<域名>:443
  api_key: "<你的 LiveKit key>"
  api_secret: "<你的 LiveKit secret>"
redis:
  addr: "127.0.0.1:6379"
  password: "<你的 Redis 口令>"
  db: 0
  pin_ttl_seconds: 86400
```

防火墙 / 安全组需放行：

- `80/tcp`（ACME 验证）
- `443/tcp`（H5 + WSS）
- `7880/tcp`（信令，若 nginx 与 LiveKit 同机且仅本机反代，可不对外）
- `7881/tcp`、`7882/udp`（WebRTC 媒体）

### 发布更新

```bash
sudo systemctl restart h5room
```

### 冒烟测试（服务器本机）

```bash
curl -s -X POST http://127.0.0.1:3000/api/v1/room/create \
  -H 'Content-Type: application/json' -d '{"mode":"voice"}'

# 用返回的 room/pin 换 token，验证 JWT 中 roomConfig.max_participants（语音 10 / 视频 5）
curl -s -X POST http://127.0.0.1:3000/api/v1/token \
  -H 'Content-Type: application/json' \
  -d '{"room":"<room>","name":"t","join_pin":"<pin>"}'

# LiveKit 验签
curl -s -X POST http://127.0.0.1:7880/rtc/validate \
  -H "Authorization: Bearer <token>"
```

## HTTPS 测试（自签名证书）

测试期没有正式证书时，可用仓库脚本生成自签名证书，由 nginx 统一处理：

```bash
# 1. 生成自签名证书（IP 换成服务器公网 IP）
bash scripts/gen_selfsigned_cert.sh <公网IP>

# 2. 证书放到 nginx 目录
sudo mkdir -p /etc/nginx/ssl
sudo cp certs/cert.pem /etc/nginx/ssl/server.crt
sudo cp certs/key.pem /etc/nginx/ssl/server.key

# 3. nginx 配置 (443 ssl):
#    /rtc → http://127.0.0.1:7880 (带 Upgrade/Connection 头)
#    /    → http://127.0.0.1:3000 (H5 页面)
#    建议两个 location 都加 X-Forwarded-For 以记录真实客户端 IP
sudo nginx -t && sudo systemctl reload nginx

# 4. 启动基础设施 + Go 后端
docker compose up -d
go run ./cmd/server -config config.yaml
```

先 `curl -X POST https://<公网IP>/api/v1/room/create` 建房，再访问
`https://<公网IP>/?room=<房间号>&pin=<PIN>`，点过一次证书警告即可。

> `ws_url` 配置为 `wss://127.0.0.1:443` 时，服务端会按请求 Host 自动改写为
> `wss://<IP>:443`。本地开发（`localhost:5173`）请改回 `ws://127.0.0.1:7880`。
> 桌面 Chrome/Edge、Android Chrome 可点过警告使用；**iOS Safari 需安装证书描述文件**；
> App/微信内 WebView 无法接受自签名证书，只能使用正式证书。

## 使用流程

1. **生成链接** — App 调 `POST /api/v1/room/create` 拿到房间号 + PIN，拼 URL：
   ```
   https://your-host/?room=<房间号>&pin=<口令>&mode=<video|voice>
   ```
   - `mode` 缺省 `video`；`voice` = 语音房（服务端强制仅麦克风）
   - 房间模式由创建时定死并存入 Redis，后续请求以服务端返回为准
   - 绑定 `user_addr` 的房间，进房者必须通过钱包地址校验；地址由 App WebView bridge
     注入或浏览器扩展钱包提供（见下节）
   - 可带 `&name=<昵称>` 自动填入昵称；room + pin + name 齐全时自动进房

2. **打开 H5** — App 内或外部浏览器打开链接，表单自动预填房间号和口令
   - 输入昵称，点击「登录获取钱包地址」（App 内自动注入，浏览器需 MetaMask 等扩展）
   - 点击「加入房间」— 服务端校验钱包地址与建房绑定值一致才放行

3. **进入房间** — 连接 LiveKit
   - **视频房**：自动开启摄像头 + 麦克风，5 人 3+2 网格；快捷键 `M` 麦克风 / `V` 摄像头
   - **语音房**：仅麦克风，无摄像头按钮；参与者列表实时显示，说话者有绿色光圈
   - 人数上限：视频房 5 人，语音房 10 人

4. **挂断** — 点击挂断按钮回到加入页面

## App 内嵌 WebView 接入规范

H5 会在 App 的「发现」页内嵌打开。WebView 对 WebRTC 支持参差，需要 App 侧配合：

| 平台 | App 必须做的配置 | 备注 |
|------|----------------|------|
| Android WebView | `WebChromeClient.onPermissionRequest` 放行摄像头/麦克风 | 默认**拒绝**，未实现则 H5 永远拿不到设备 |
| iOS WKWebView | Info.plist 声明 `NSCameraUsageDescription` / `NSMicrophoneUsageDescription` | 无声明直接崩溃或拒绝 |
| 全部 | 页面必须 **HTTPS** | http 下 `getUserMedia` 被浏览器禁用（iOS ATS 亦强制） |
| 全部 | `mediaPlaybackRequiresUserGesture=false`（iOS: `mediaTypesRequiringUserActionForPlayback=[]`）| 不配则进房**黑屏 + 无声**，必须点一次屏幕 |
| 全部 | 注入钱包地址 bridge `window.H5Wallet.getAddress()` | 进房身份校验必需，见下 |

**钱包地址 bridge 约定**：App 在 WebView 加载 H5 后注入

```js
// Android: addJavascriptInterface 注入对象; iOS: WKUserScript 注入
window.H5Wallet = {
  getAddress: () => Promise.resolve('0x用户钱包地址')  // 已登录用户的钱包地址
}
```

- H5 进房时优先调用该 bridge 获取地址；App 未注入时回退浏览器扩展钱包（MetaMask 等 `window.ethereum`）
- 进房请求携带该地址，服务端与建房绑定的 `user_addr` 比对，不一致拒绝（`user address not allowed`）
- 地址由 App 从自身登录态提供，**不需要** H5 侧再走钱包签名

已知限制：

- **切后台/锁屏**：H5 音频由浏览器控制，App 切后台或锁屏后音频可能中断（iOS 尤甚）。
  这是 H5 方案的天花板，语音场景请提示用户保持前台。
- **不支持 WebView 时**：加入页提示「复制链接到浏览器打开」，用户可在外部浏览器完成通话。

App 发现页跳转建议带上房间参数：
`https://your-host/?room=xxx&pin=xxx&mode=voice&name=xxx`

## Token 签发日志

每次 `POST /api/v1/token` 与 `GET /api/v1/room/pin-status` 请求都会记录到后端 stdout
（日志文件 / journald）：

```
api: POST /api/v1/token → 200 (12.3ms) remote=1.2.3.4 | req={"room":"voice1",...} | resp={"code":0,...,"token":"eyJ..."}
```

- 含请求/响应体（截断 300 字符）、客户端 IP、耗时
- 响应的 `token` 即当次签发的 JWT（默认 2 小时有效），会随日志留存
- 日志包含短期凭证与用户地址，生产环境请限制日志访问权限并设置合理轮转
- 静态资源不记录

## API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/token` | POST | 签发 LiveKit 接入令牌 |
| `/api/v1/room/create` | POST | 服务端生成随机房间号 + PIN（App 建房入口） |
| `/api/v1/room/owners` | GET | 房主列表（0x 地址 + 最近建房时间） |
| `/api/v1/room/pin-status` | GET | 查询房间口令状态 |
| `/` | GET | 静态文件（Vue 生产构建） |

### POST /api/v1/room/create

**App 端建房流程**：先调本接口拿到房间号 + PIN，再拼接入链接发给用户。
校验、撞号重试、PIN 限流都在服务端完成。

```json
// Request
{
  "mode": "voice",              // 可选: "voice"; 缺省 video
  "owner": "0xAbC...9f3a",      // 可选: 房主 0x 地址 (保存入房主列表)
  "user_addr": "0x1234...abcd"  // 可选: 目标用户 0x 地址 (绑定后进房时校验一致)
}

// Response (success)
{ "code": 0, "room": "h5_voice_abc234", "pin": "8866", "mode": "voice", "owner": "0xAbC...9f3a", "user_addr": "0x1234...abcd" }

// Response (error)
{ "code": 1, "error": "..." }
```

- 房间号 = `h5_`（视频）/ `h5_voice_`（语音）+ 6 位随机码（字符集去掉易混淆的
  `0/1/o/i/l`），PIN 为 4 位数字；前缀用于与其他服务的 LiveKit 房间隔离
- App 拼链接：`https://your-host/?room=h5_voice_abc234&pin=8866&mode=voice&name=<昵称>`
  （**不带** `user_addr` —— 进房身份由钱包登录提供）
- PIN 防爆破：同一房间连续错 5 次锁定 10 分钟（Redis key `h5room:pinfail:<room>`）
- **`user_addr` 绑定**：0x 地址（42 字符，统一小写存储）；提供后进房者必须钱包登录
  获取一致地址，否则 `user address not allowed`；不传则不校验
- **`owner`**：0x 地址，提供后存入房主列表（`h5room:owners`，记录最近建房时间）

### GET /api/v1/room/owners

```json
// Response
{ "code": 0, "owners": [ { "addr": "0xabc...", "last_created_at": 1724567890 } ] }
```

- 按最近建房时间倒序；`last_created_at` 为 unix 秒
- 同一房主多次建房只保留一条（时间更新为最新）

### POST /api/v1/token

```json
// Request
{ "room": "test01", "name": "Alice", "join_pin": "1234", "mode": "voice", "user_addr": "0x1234...abcd" }

// Response (success)
{ "code": 0, "room": "test01", "token": "eyJ...", "ws_url": "ws://...", "is_owner": false, "mode": "voice" }

// Response (error)
{ "code": 1, "error": "wrong pin" }
{ "code": 1, "error": "room not found" }             // 房间不存在 (严格模式: 必须先 create)
{ "code": 1, "error": "user address not allowed" }   // 房间绑定了 user_addr, 地址不一致
```

- `identity` 可不传 —— 服务端自动生成唯一 identity，避免同昵称用户互相挤下线
- 房间模式由 `create` 接口建房时定死；响应 `mode` 是服务端权威值
  （语音房强制 `canPublishSources: ["microphone"]`）
- 严格模式：房间必须先创建，链接无法隐式建房；PIN 错误连续 5 次锁定 10 分钟
- 人数上限随 token 下发：语音房 10 人、视频房 5 人

## 安全建议

- 不要提交 `config.yaml`、`.env`、证书私钥或任何真实密钥
- 生产环境更换 LiveKit `api_key/api_secret` 与 Redis 口令，使用随机强口令
- Go 后端默认只监听 `127.0.0.1:3000`（配置以 `:` 开头时也会绑定 loopback），对外统一走 nginx
- 限制 API 日志访问权限；日志中会包含短期有效的 LiveKit JWT

## License

[MIT](LICENSE)
