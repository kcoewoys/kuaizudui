import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import type { InviteActivityConfig } from '@/domain/activities'
import { api, friendlyApiError, subscribeActivityUpdates, type ActivityStateResponse } from '@/services/api'
import { loadUser, userState } from '@/services/session'
import { copyText } from '@/utils/clipboard'

export function useInviteActivity(config: InviteActivityConfig) {
  const state = reactive({
    content: '',
    publishedAt: null as string | null,
    updatedAt: null as string | null,
    ordinaryRounds: 0,
    ordinaryCredit: 0,
    priorityRounds: 0,
    pointsCommitted: 0,
    priorityCredit: 0,
    claimCount: 0,
    canClaim: false,
    loading: true,
    working: false,
  })
  const draft = ref('')
  const boostPointsInput = ref('1')
  const notice = ref(config.intro)
  const error = ref('')
  const toast = ref('')
  let toastTimer: number | undefined
  let eventsAbortController: AbortController | undefined
  let reconnectTimer: number | undefined
  let reconnectDelay = 1000
  let stopped = false
  let refreshRequestPending = false
  let refreshQueued = false
  let stateRevision = 0

  const remaining = computed(() => 200 - draft.value.length)
  // 普通队列机会总量 = 已被领取次数 + 剩余次数（发布赠送 3 次 + 领码攒出的机会）。
  const ordinaryQuota = computed(() => state.ordinaryRounds + state.ordinaryCredit)
  const isPublished = computed(() => Boolean(state.content))
  const userPoints = computed(() => Math.max(0, userState.points))
  const maxBoostPoints = computed(() => userPoints.value)
  const selectedBoostPoints = computed(() => {
    const value = Number.parseInt(boostPointsInput.value, 10)
    return Number.isFinite(value) ? value : 0
  })
  const canDecreaseBoostPoints = computed(() => !state.working && selectedBoostPoints.value > 1)
  const canIncreaseBoostPoints = computed(() => !state.working && selectedBoostPoints.value < maxBoostPoints.value)
  const canUseBoost = computed(() => (
    !state.working
    && selectedBoostPoints.value >= 1
    && selectedBoostPoints.value <= maxBoostPoints.value
  ))
  const maskedContent = computed(() => {
    const characters = Array.from(state.content)
    const edgeLength = Math.min(4, Math.floor(characters.length / 2))
    if (edgeLength === 0) return characters.length ? `${characters[0]}***` : ''
    return `${characters.slice(0, edgeLength).join('')}***${characters.slice(-edgeLength).join('')}`
  })
  const recordTimestamp = computed(() => state.updatedAt || state.publishedAt || '')
  const recordTimestampLabel = computed(() => {
    if (!state.updatedAt || !state.publishedAt) return '发布于'
    return new Date(state.updatedAt).getTime() > new Date(state.publishedAt).getTime() ? '更新于' : '发布于'
  })

  watch(draft, () => {
    if (draft.value.length <= 200) error.value = ''
  })

  function applyActivity(value: ActivityStateResponse, preserveDraft = false) {
    const shouldSyncDraft = !preserveDraft || draft.value === state.content
    state.content = value.content
    state.publishedAt = value.published_at || null
    state.updatedAt = value.updated_at || null
    state.ordinaryRounds = value.ordinary_rounds
    state.ordinaryCredit = value.ordinary_credit
    state.priorityRounds = value.priority_rounds
    state.pointsCommitted = value.points_committed
    state.priorityCredit = value.priority_credit
    state.claimCount = value.claim_count
    state.canClaim = value.can_claim
    if (shouldSyncDraft) draft.value = value.content
  }

  function showToast(message: string) {
    toast.value = message
    window.clearTimeout(toastTimer)
    toastTimer = window.setTimeout(() => (toast.value = ''), 2200)
  }

  async function load() {
    state.loading = true
    error.value = ''
    try {
      const [, activity, configuredNotice] = await Promise.all([
        loadUser(),
        api.activity.detail(config.type),
        api.public.notice(config.type).catch(() => null),
      ])
      applyActivity(activity)
      if (configuredNotice?.content) notice.value = configuredNotice.content
    } catch (loadError) {
      error.value = friendlyApiError(loadError)
    } finally {
      state.loading = false
    }
  }

  async function refreshStatus() {
    if (state.loading || !isPublished.value) return
    if (state.working || refreshRequestPending) {
      refreshQueued = true
      return
    }
    refreshRequestPending = true
    const revisionAtStart = stateRevision
    try {
      const activity = await api.activity.detail(config.type)
      if (revisionAtStart !== stateRevision || state.working) return
      applyActivity(activity, true)
    } catch {
      // Keep the current values; the event connection will retry independently.
    } finally {
      refreshRequestPending = false
      if (refreshQueued) {
        refreshQueued = false
        void refreshStatus()
      }
    }
  }

  async function publish() {
    const value = draft.value.trim()
    if (!value) {
      error.value = '请先粘贴邀请内容'
      return false
    }
    if (value.length > 200) {
      error.value = '邀请内容不能超过 200 字'
      return false
    }
    const wasPublished = isPublished.value
    stateRevision += 1
    state.working = true
    try {
      applyActivity(await api.activity.publish(config.type, value))
      showToast(wasPublished ? '内容已保存' : '发布成功')
      return true
    } catch (publishError) {
      error.value = friendlyApiError(publishError)
      return false
    } finally {
      state.working = false
    }
  }

  function normalizeBoostPoints() {
    boostPointsInput.value = String(Math.max(1, selectedBoostPoints.value || 1))
  }

  function adjustBoostPoints(delta: number) {
    const upperBound = Math.max(1, maxBoostPoints.value)
    const current = selectedBoostPoints.value || 1
    boostPointsInput.value = String(Math.min(upperBound, Math.max(1, current + delta)))
  }

  async function useBoost() {
    if (state.working) return
    const points = selectedBoostPoints.value
    if (points < 1) {
      showToast('请输入大于 0 的整数积分')
      return
    }
    if (maxBoostPoints.value < 1) {
      showToast('积分不足，暂时无法插队')
      return
    }
    if (points > maxBoostPoints.value) {
      showToast(`本次最多可使用 ${maxBoostPoints.value} 积分`)
      return
    }
    stateRevision += 1
    state.working = true
    try {
      applyActivity(await api.activity.boost(config.type, points))
      await loadUser(true)
      boostPointsInput.value = '1'
      showToast(`已使用 ${points} 积分，已加入插队队列`)
    } catch (boostError) {
      showToast(friendlyApiError(boostError))
    } finally {
      state.working = false
    }
  }

  async function copyContent() {
    if (!state.content || !state.canClaim || state.working) return
    stateRevision += 1
    state.working = true
    try {
      const result = await api.activity.use(config.type)
      applyActivity(result.state)
      await copyText(result.content)
      showToast(result.source === 'priority' ? '已优先领码并复制' : '邀请内容已复制')
    } catch (copyError) {
      showToast(friendlyApiError(copyError))
    } finally {
      state.working = false
    }
  }

  function disconnectActivityEvents() {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = undefined
    const controller = eventsAbortController
    eventsAbortController = undefined
    controller?.abort()
  }

  function connectActivityEvents() {
    if (stopped || document.visibilityState !== 'visible' || eventsAbortController) return
    const controller = new AbortController()
    eventsAbortController = controller
    void subscribeActivityUpdates((event) => {
      reconnectDelay = 1000
      if (event.type === config.type) void refreshStatus()
    }, controller.signal, () => {
      reconnectDelay = 1000
      void refreshStatus()
    }).catch(() => {
      // Reconnection is scheduled below unless the page is hidden or unmounted.
    }).finally(() => {
      if (eventsAbortController === controller) eventsAbortController = undefined
      if (stopped || document.visibilityState !== 'visible' || eventsAbortController) return
      const delay = reconnectDelay
      reconnectDelay = Math.min(reconnectDelay * 2, 15_000)
      reconnectTimer = window.setTimeout(connectActivityEvents, delay)
    })
  }

  function handleVisibilityChange() {
    if (document.visibilityState === 'visible') {
      connectActivityEvents()
      return
    }
    disconnectActivityEvents()
  }

  onMounted(() => {
    void load().then(connectActivityEvents)
    document.addEventListener('visibilitychange', handleVisibilityChange)
  })

  onBeforeUnmount(() => {
    stopped = true
    window.clearTimeout(toastTimer)
    disconnectActivityEvents()
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  })

  return {
    state,
    draft,
    notice,
    error,
    toast,
    remaining,
    ordinaryQuota,
    isPublished,
    boostPointsInput,
    userPoints,
    maxBoostPoints,
    canDecreaseBoostPoints,
    canIncreaseBoostPoints,
    canUseBoost,
    maskedContent,
    recordTimestamp,
    recordTimestampLabel,
    load,
    refreshStatus,
    publish,
    normalizeBoostPoints,
    adjustBoostPoints,
    useBoost,
    copyContent,
  }
}
