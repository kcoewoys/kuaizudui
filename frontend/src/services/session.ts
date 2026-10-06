import { reactive } from 'vue'
import { api, setReferralGate, waitForReferral, type UserInfo } from './api'

export const userState = reactive({
  uid: '',
  phone: '',
  inviteCode: '',
  invitedByPhone: '',
  points: 0,
  firstVisit: false,
  loading: false,
  loaded: false,
})

let pendingLoad: Promise<typeof userState> | null = null

function applyUser(user: UserInfo) {
  userState.uid = user.uid
  userState.phone = user.phone || ''
  userState.inviteCode = user.invite_code || ''
  userState.invitedByPhone = user.invited_by_phone || ''
  userState.points = user.points
  userState.firstVisit = user.first_visit
  userState.loaded = true
  return userState
}

export async function loadUser(force = false) {
  if (userState.loaded && !force) return userState
  if (pendingLoad) return pendingLoad
  await waitForReferral()
  if (userState.loaded && !force) return userState
  if (pendingLoad) return pendingLoad
  userState.loading = true
  pendingLoad = api.user.info().then(applyUser).finally(() => {
    userState.loading = false
    pendingLoad = null
  })
  return pendingLoad
}

export async function bindUserPhone(phone: string) {
  return applyUser(await api.user.bindPhone(phone))
}

let referralPending: Promise<void> | null = null

export function captureReferralFromUrl() {
  // 只识别路径里的邀请码（/r/<code>）。绑定成功后不要把邀请码从地址栏
  // 移除：微信「用浏览器打开」传递的是当前 URL，浏览器侧是全新身份
  // （localStorage 不共享），需要靠 URL 里的邀请码再绑定一次，否则邀请
  // 关系会断在微信 webview 里。
  const pathMatch = window.location.pathname.match(/^\/r\/([A-Za-z0-9]{4,16})/)
  if (!pathMatch) return Promise.resolve()
  if (referralPending) return referralPending

  const request = api.user.applyReferral(pathMatch[1]).then(applyUser)
  setReferralGate(request)
  // 邀请归因是尽力而为：链接失效（404）等失败不弹错、不产生未处理拒绝
  referralPending = request
    .then(
      () => undefined,
      () => undefined,
    )
    .finally(() => {
      referralPending = null
      setReferralGate(null)
    })
  return referralPending
}

export function applyPoints(points: number) {
  userState.points = points
}
