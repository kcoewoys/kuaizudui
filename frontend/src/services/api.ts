import type { InviteActivityType } from '@/domain/activities'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
const uidStorageKey = 'eaok:uid'

interface ApiEnvelope<T> {
  code: number | string
  message: string
  data: T
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export interface UserInfo {
  uid: string
  phone?: string
  invited_by_phone?: string
  points: number
  first_visit: boolean
}

export interface ActivityStateResponse {
  type: InviteActivityType
  content: string
  published: boolean
  ordinary_rounds: number
  priority_rounds: number
  points_committed: number
  priority_credit: number
  claim_count: number
  can_claim: boolean
  published_at?: string
  updated_at?: string
}

export interface ActivityUseResponse {
  content: string
  source: 'priority' | 'ordinary'
  state: ActivityStateResponse
}

export interface ActivityUpdateEvent {
  type: InviteActivityType
}

export interface LuckyListItem {
  id: number
  masked_code: string
  source: string
  is_own: boolean
  created_at: string
}

export interface LuckyResult {
  id: number
  code: string
  created_at: string
}

export interface PointRecord {
  id: number
  uid: string
  source: string
  description: string
  points: number
  created_at: string
}

export interface ExchangeResult {
  awarded_points: number
  total_points: number
}

export interface FeedbackRecord {
  id: number
  uid: string
  content: string
  created_at: string
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const uid = window.localStorage.getItem(uidStorageKey)
  if (uid) headers.set('X-UID', uid)

  let response: Response
  try {
    response = await fetch(`${apiBase}${path}`, { ...init, headers })
  } catch {
    throw new ApiError('无法连接服务器，请稍后重试', 0, 'network_error')
  }

  const responseUID = response.headers.get('X-UID')
  if (responseUID) window.localStorage.setItem(uidStorageKey, responseUID)

  let payload: ApiEnvelope<T> | undefined
  try {
    payload = (await response.json()) as ApiEnvelope<T>
  } catch {
    if (!response.ok) throw new ApiError('服务器返回了无效响应', response.status, 'invalid_response')
  }
  if (!response.ok || !payload || payload.code !== 0) {
    throw new ApiError(payload?.message || '请求失败', response.status, String(payload?.code || 'request_failed'))
  }
  return payload.data
}

const json = (body: unknown): RequestInit => ({ method: 'POST', body: JSON.stringify(body) })

export function userSessionUID() {
  return window.localStorage.getItem(uidStorageKey) || ''
}

export async function subscribeActivityUpdates(
  onUpdate: (event: ActivityUpdateEvent) => void,
  signal: AbortSignal,
  onConnected?: () => void,
) {
  const headers = new Headers({ Accept: 'text/event-stream' })
  const uid = userSessionUID()
  if (uid) headers.set('X-UID', uid)

  const response = await fetch(`${apiBase}/activity/events`, {
    headers,
    signal,
    cache: 'no-store',
  })
  const responseUID = response.headers.get('X-UID')
  if (responseUID) window.localStorage.setItem(uidStorageKey, responseUID)
  if (!response.ok || !response.body) {
    throw new ApiError('实时更新连接失败', response.status, 'activity_events_unavailable')
  }
  onConnected?.()

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) throw new ApiError('实时更新连接已断开', 0, 'activity_events_closed')
    buffer = (buffer + decoder.decode(value, { stream: true })).replace(/\r/g, '')
    let boundary = buffer.indexOf('\n\n')
    while (boundary >= 0) {
      const block = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      const data = block
        .split('\n')
        .filter((line) => line.startsWith('data:'))
        .map((line) => line.slice(5).trimStart())
        .join('\n')
      if (data) {
        try {
          onUpdate(JSON.parse(data) as ActivityUpdateEvent)
        } catch {
          // Ignore malformed events and keep the stream connected.
        }
      }
      boundary = buffer.indexOf('\n\n')
    }
  }
}

export function resolveApiAssetUrl(value: string) {
  if (!value || /^[a-z][a-z\d+.-]*:/i.test(value)) return value
  const apiOrigin = new URL(`${apiBase}/`, window.location.origin)
  return new URL(value, apiOrigin).toString()
}

export const api = {
  user: {
    info: () => request<UserInfo>('/user/info'),
    bindPhone: (phone: string) => request<UserInfo>('/user/bind-phone', json({ phone })),
    applyReferral: (phone: string) => request<UserInfo>('/user/referral', json({ phone })),
  },
  lucky: {
    list: (limit = 20) => request<{ items: LuckyListItem[]; count: number }>(`/lucky/list?limit=${limit}`),
    publish: (code: string) => request<LuckyResult>('/lucky/publish', json({ code })),
    receive: () => request<LuckyResult>('/lucky/receive', { method: 'POST' }),
    use: (id: number) => request<LuckyResult>('/lucky/use', json({ id })),
  },
  activity: {
    detail: (type: InviteActivityType) => request<ActivityStateResponse>(`/activity/detail?type=${type}`),
    publish: (type: InviteActivityType, content: string) =>
      request<ActivityStateResponse>('/activity/publish', json({ type, content })),
    boost: (type: InviteActivityType, points: number) =>
      request<ActivityStateResponse>('/activity/boost', json({ type, points })),
    use: (type: InviteActivityType) => request<ActivityUseResponse>('/activity/use', json({ type })),
  },
  points: {
    get: () => request<{ points: number }>('/points'),
    history: (limit = 20, offset = 0) =>
      request<{ items: PointRecord[] }>(`/points/history?limit=${limit}&offset=${offset}`),
    exchange: (code: string) => request<ExchangeResult>('/exchange', json({ code })),
  },
  public: {
    notice: (type: string) => request<{ type: string; content: string; updated_at?: string }>(`/notices/${type}`),
    submitFeedback: (content: string) => request<FeedbackRecord>('/feedback', json({ content })),
    async groupQRCode() {
      const result = await request<{ qrcode: string }>('/group-qrcode')
      return { ...result, qrcode: resolveApiAssetUrl(result.qrcode) }
    },
  },
}

export function friendlyApiError(error: unknown) {
  if (!(error instanceof ApiError)) return '操作失败，请稍后重试'
  if (error.code === 'queue_empty') return '暂时没有可领取的内容'
  if (error.code === 'insufficient_points') return '可用积分不足，请减少使用积分数'
  if (error.code === 'conflict') {
    if (error.message.includes('phone')) return '该手机号码已绑定'
    return '内容已被领取或重复提交'
  }
  if (error.code === 'not_found') return '没有找到对应内容'
  if (error.code === 'network_error') return error.message
  if (error.code === 'invalid_input') {
    if (error.message.includes('positive integer')) return '请输入大于 0 的整数积分'
    return error.message.split(':').slice(1).join(':').trim() || '输入内容不符合要求'
  }
  return error.message || '操作失败，请稍后重试'
}
