<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ArrowLeft, Check, Clock3, HelpCircle, RefreshCw, UserRound } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import MobileShell from '@/components/MobileShell.vue'
import AppFooter from '@/components/AppFooter.vue'
import GuideModal from '@/components/GuideModal.vue'
import ToastMessage from '@/components/ToastMessage.vue'
import { api, friendlyApiError, type LuckyListItem } from '@/services/api'
import { luckyTeamConfig } from '@/domain/activities'
import { copyText } from '@/utils/clipboard'

const config = luckyTeamConfig
const router = useRouter()
const guideOpen = ref(false)
const codeInput = ref('')
const error = ref('')
const toast = ref('')
const noticeText = ref(config.intro)
const codes = ref<LuckyListItem[]>([])
const loading = ref(true)
const working = ref(false)
const workingId = ref<number | null>(null)
const refreshCountdown = ref(10)
let toastTimer: number | undefined
let refreshTimer: number | undefined
let codesRequestPending = false

const availableCount = computed(() => codes.value.length)

function showToast(message: string) {
  toast.value = message
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = ''), 2200)
}

async function loadCodes(withToast = false) {
  if (codesRequestPending) return
  codesRequestPending = true
  if (codes.value.length === 0) loading.value = true
  try {
    const result = await api.lucky.list(50)
    codes.value = result.items
    if (withToast) showToast(config.listRefreshedToast)
  } catch (loadError) {
    showToast(friendlyApiError(loadError))
  } finally {
    loading.value = false
    codesRequestPending = false
    refreshCountdown.value = 10
  }
}

function startAutoRefresh() {
  refreshTimer = window.setInterval(() => {
    if (refreshCountdown.value > 1) {
      refreshCountdown.value -= 1
      return
    }
    refreshCountdown.value = 10
    void loadCodes()
  }, 1000)
}

async function publishCode() {
  const value = codeInput.value.trim()
  if (!/^\d{8,9}$/.test(value)) {
    error.value = config.invalidCodeError
    return
  }
  working.value = true
  try {
    await api.lucky.publish(value)
    codeInput.value = ''
    error.value = ''
    showToast(config.publishedToast)
    await loadCodes()
  } catch (publishError) {
    error.value = friendlyApiError(publishError)
  } finally {
    working.value = false
  }
}

async function useCode(item: LuckyListItem) {
  if (item.is_own || workingId.value !== null) return
  workingId.value = item.id
  try {
    const result = await api.lucky.use(item.id)
    await copyText(result.code)
    codes.value = codes.value.filter((candidate) => candidate.id !== item.id)
    showToast(config.usedToast)
  } catch (useError) {
    showToast(friendlyApiError(useError))
    await loadCodes()
  } finally {
    workingId.value = null
  }
}

async function receiveOne() {
  if (working.value) return
  working.value = true
  try {
    const result = await api.lucky.receive()
    await copyText(result.code)
    codes.value = codes.value.filter((candidate) => candidate.id !== result.id)
    showToast(config.receivedToast)
  } catch (receiveError) {
    showToast(friendlyApiError(receiveError))
  } finally {
    working.value = false
  }
}

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

onMounted(() => {
  startAutoRefresh()
  void Promise.allSettled([
    loadCodes(),
    api.public.notice('lucky').then((notice) => {
      if (notice.content) noticeText.value = notice.content
    }),
  ])
})

onBeforeUnmount(() => {
  window.clearTimeout(toastTimer)
  window.clearInterval(refreshTimer)
})
</script>

<template>
  <MobileShell>
    <header class="page-header">
      <button class="icon-button" type="button" aria-label="返回" @click="router.push('/')"><ArrowLeft :size="21" /></button>
      <h1>{{ config.title }}</h1>
      <button class="profile-button profile-button--soft" type="button" aria-label="个人中心" @click="router.push('/profile')"><UserRound :size="18" /></button>
    </header>

    <section class="info-panel">
      <p>{{ noticeText }}</p>
    </section>

    <button class="guide-button" type="button" @click="guideOpen = true">
      <HelpCircle :size="17" />
      <span>{{ config.guideButton }}</span>
    </button>

    <section class="publish-panel publish-panel--inline card-surface">
      <input
        v-model="codeInput"
        class="text-input lucky-input"
        inputmode="numeric"
        maxlength="9"
        :placeholder="config.placeholder"
        :aria-label="config.inputLabel"
        :disabled="working"
        @input="error = ''"
        @keyup.enter="publishCode"
      />
      <button class="primary-button publish-button" :disabled="working" type="button" @click="publishCode">
        {{ working ? config.publishingButton : config.publishButton }}
      </button>
      <p v-if="error" class="field-error publish-error">{{ error }}</p>
    </section>

    <section class="code-panel card-surface">
      <div class="code-panel-heading">
        <h2>{{ config.listTitle }} <b>{{ availableCount }}</b></h2>
        <span class="status-chip" :aria-label="`${refreshCountdown} 秒后自动刷新`">
          <span />{{ refreshCountdown }}s后刷新
        </span>
      </div>
      <div class="code-actions">
        <button class="secondary-button" :disabled="loading" type="button" @click="loadCodes(true)"><RefreshCw :size="16" />{{ config.refreshButton }}</button>
        <button class="primary-button" :disabled="working" type="button" @click="receiveOne">{{ config.receiveButton }}</button>
      </div>

      <div v-if="loading" class="code-loading" :aria-label="config.loadingLabel">
        <div v-for="index in 4" :key="index" class="skeleton-line" />
      </div>
      <p v-else-if="codes.length === 0" class="list-empty">{{ config.emptyText }}</p>
      <ul v-else class="code-list">
        <li v-for="item in codes" :key="item.id" :class="{ 'code-item--mine': item.is_own }">
          <div>
            <strong>{{ item.masked_code }}</strong>
            <small>
              <span><Clock3 :size="12" />{{ formatTime(item.created_at) }}</span>
              <span class="code-source">from {{ item.source }}</span>
            </small>
          </div>
          <button
            type="button"
            :aria-label="item.is_own ? `${item.masked_code} 是本人发布，不可使用` : `使用福袋码 ${item.masked_code}`"
            :disabled="item.is_own || workingId !== null"
            @click="useCode(item)"
          >
            <Check :size="14" />{{ workingId === item.id ? config.usingButton : config.useButton }}
          </button>
        </li>
      </ul>
    </section>

    <AppFooter />
    <GuideModal
      :open="guideOpen"
      :title="config.guideTitle"
      :steps="config.guideSteps"
      :note="config.guideNote"
      @close="guideOpen = false"
    />
    <ToastMessage :message="toast" />
  </MobileShell>
</template>
