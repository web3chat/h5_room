/**
 * Backend API calls for h5_room.
 */
const BASE = ''

/**
 * Request a LiveKit access token for a room.
 * @param {{ room: string, identity: string, name: string, join_pin: string, mode?: string, user_addr?: string }} params
 *   user_addr: 进房者 0x 地址; 房间绑定了 user_addr 时必填且必须一致
 * @returns {Promise<{ code: number, token?: string, ws_url?: string, is_owner?: boolean, mode?: string, error?: string }>}
 */
export async function fetchToken({ room, identity, name, join_pin, mode, user_addr }) {
  const resp = await fetch(`${BASE}/api/v1/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ room, identity, name, join_pin, mode, user_addr }),
  })
  return resp.json()
}

/**
 * Check whether a room requires a PIN for entry.
 * @param {string} room
 * @returns {Promise<{ code: number, pin_set?: boolean, found?: boolean }>}
 */
export async function fetchPinStatus(room) {
  const resp = await fetch(`${BASE}/api/v1/room/pin-status?room=${encodeURIComponent(room)}`)
  return resp.json()
}

/**
 * Create a room on the server; returns the generated room id, PIN and mode.
 * 供 App 端集成: 创建后拼接入链接发给用户。
 * @param {string} [mode] 'voice' or omitted for video
 * @returns {Promise<{ code: number, room?: string, pin?: string, mode?: string, error?: string }>}
 */
export async function createRoom(mode) {
  const resp = await fetch(`${BASE}/api/v1/room/create`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  })
  return resp.json()
}
