import { ApiError, resolveApiAssetUrl } from '@/services/api'

const apiBase = (import.meta.env.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
let adminToken = ''

interface ApiEnvelope<T> {
  code: number | string
  message: string
  data?: T
}

export interface AdminSession {
  token: string
  expires_at: string
}

export interface AdminUser {
  uid: string
  phone?: string
  points: number
  created_at: string
  updated_at: string
}

export interface RechargeRecord {
  id: number
  phone: string
  points: number
  admin_phone: string
  created_at: string
}

export interface ExchangeCodeRecord {
  id: number
  code: string
  points: number
  status: 'unused' | 'used'
  used_uid?: string
  created_at: string
  used_at?: string
}

export interface NoticeRecord {
  type: string
  content: string
  updated_at?: string
}

export interface AdminFeedbackRecord {
  id: number
  uid: string
  phone?: string
  content: string
  created_at: string
}

export interface QueueStatus {
  created: boolean
  total: number
  position: number
  cursor_seq: number
}

export interface ActivityQueueSnapshot {
  type: 'buy_food' | 'cash_turntable' | 'cash_monopoly' | 'daily_cash'
  ordinary: QueueStatus
  priority: QueueStatus
}

async function request<T>(path: string, init: RequestInit = {}, requireSession = true): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (init.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  if (requireSession && adminToken) headers.set('Authorization', `Bearer ${adminToken}`)

  let response: Response
  try {
    response = await fetch(`${apiBase}${path}`, { ...init, headers })
  } catch {
    throw new ApiError('无法连接服务器，请稍后重试', 0, 'network_error')
  }

  let payload: ApiEnvelope<T> | undefined
  try {
    payload = (await response.json()) as ApiEnvelope<T>
  } catch {
    if (!response.ok) throw new ApiError('服务器返回了无效响应', response.status, 'invalid_response')
  }

  if (!response.ok || !payload || payload.code !== 0 || payload.data === undefined) {
    const code = String(payload?.code || 'request_failed')
    if (code === 'unauthorized') adminToken = ''
    throw new ApiError(payload?.message || '请求失败', response.status, code)
  }
  return payload.data
}

const post = (body?: unknown): RequestInit => ({
  method: 'POST',
  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
})

export const adminApi = {
  async verifyPhone(phone: string) {
    const session = await request<AdminSession>('/admin/login', post({ phone }), false)
    adminToken = session.token
    return session
  },
  async logout() {
    try {
      if (adminToken) await request<{ logged_out: boolean }>('/admin/logout', post())
    } finally {
      adminToken = ''
    }
  },
  clearSession() {
    adminToken = ''
  },
  recharge(phone: string, points: number) {
    return request<AdminUser>('/admin/recharge', post({ phone, points }))
  },
  recharges(limit = 20, offset = 0) {
    return request<{ items: RechargeRecord[] }>(`/admin/recharges?limit=${limit}&offset=${offset}`)
  },
  createExchangeCodes(points: number, count: number, prefix: string) {
    return request<{ items: ExchangeCodeRecord[]; count: number }>('/admin/exchange/create', post({ points, count, prefix }))
  },
  exchangeCodes(status = '', limit = 50, offset = 0) {
    const params = new URLSearchParams({ status, limit: String(limit), offset: String(offset) })
    return request<{ items: ExchangeCodeRecord[] }>(`/admin/exchanges?${params}`)
  },
  feedback(limit = 50, offset = 0) {
    return request<{ items: AdminFeedbackRecord[] }>(`/admin/feedback?limit=${limit}&offset=${offset}`)
  },
  activityQueues() {
    return request<{ items: ActivityQueueSnapshot[] }>('/admin/activity-queues')
  },
  dailyReset() {
    return request<{ reset: boolean }>('/admin/daily-reset', post())
  },
  setNotice(type: string, content: string) {
    return request<NoticeRecord>('/admin/notice', post({ type, content }))
  },
  notice(type: string) {
    return request<NoticeRecord>(`/notices/${encodeURIComponent(type)}`, {}, false)
  },
  async uploadGroupQRCode(file: File) {
    const body = new FormData()
    body.append('image', file)
    const result = await request<{ qrcode: string }>('/admin/qrcode', { method: 'POST', body })
    return { ...result, qrcode: resolveApiAssetUrl(result.qrcode) }
  },
  removeGroupQRCode() {
    return request<{ qrcode: string }>('/admin/qrcode', { method: 'DELETE' })
  },
  async groupQRCode() {
    const result = await request<{ qrcode: string; max_upload_bytes: number }>('/group-qrcode', {}, false)
    return { ...result, qrcode: resolveApiAssetUrl(result.qrcode) }
  },
}

export function friendlyAdminError(error: unknown) {
  if (!(error instanceof ApiError)) return '操作失败，请稍后重试'
  if (error.code === 'unauthorized') return '该手机号没有管理员权限'
  if (error.code === 'network_error') return error.message
  if (error.code === 'not_found') return '没有找到对应记录，请确认手机号已绑定'
  if (error.code === 'invalid_input') return '输入内容不符合要求，请检查后重试'
  return error.message || '操作失败，请稍后重试'
}

export function isAdminUnauthorized(error: unknown) {
  return error instanceof ApiError && error.code === 'unauthorized'
}
