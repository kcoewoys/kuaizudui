import { reactive } from 'vue'
import { api, setReferralGate, waitForReferral, type UserInfo } from './api'

export const userState = reactive({
  uid: '',
  phone: '',
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
  const url = new URL(window.location.href)
  const inviterPhone = url.searchParams.get('ref')?.trim() || ''
  if (!/^1\d{10}$/.test(inviterPhone)) return Promise.resolve()
  if (referralPending) return referralPending

  const request = api.user.applyReferral(inviterPhone)
    .then(applyUser)
    .then(() => {
      url.searchParams.delete('ref')
      window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`)
    })
  setReferralGate(request)
  referralPending = request.finally(() => {
    referralPending = null
    setReferralGate(null)
  })
  return referralPending
}

export function applyPoints(points: number) {
  userState.points = points
}
