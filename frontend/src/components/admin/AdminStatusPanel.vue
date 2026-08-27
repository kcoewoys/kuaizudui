<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity } from 'lucide-vue-next'
import { activityConfigs } from '@/domain/activities'
import { adminApi, friendlyAdminError, isAdminUnauthorized, type ActivityQueueSnapshot, type QueueStatus } from '@/services/adminApi'

const emit = defineEmits<{
  sessionExpired: []
  message: [text: string]
}>()

const REFRESH_SECONDS = 10

const queues = ref<ActivityQueueSnapshot[]>([])
const loading = ref(true)
const loadError = ref('')
const countdown = ref(REFRESH_SECONDS)
const resetting = ref(false)
let refreshTimer: number | undefined

const lanes = [
  { key: 'ordinary', label: '普通队列游标' },
  { key: 'priority', label: '插队队列' },
] as const

function statusOf(item: ActivityQueueSnapshot, lane: (typeof lanes)[number]): QueueStatus {
  return lane.key === 'ordinary' ? item.ordinary : item.priority
}

async function loadQueues() {
  loadError.value = ''
  try {
    queues.value = (await adminApi.activityQueues()).items
  } catch (error) {
    if (isAdminUnauthorized(error)) {
      emit('sessionExpired')
      return
    }
    loadError.value = friendlyAdminError(error)
  } finally {
    loading.value = false
  }
}

function refreshNow() {
  countdown.value = REFRESH_SECONDS
  loadQueues()
}

async function resetDaily() {
  if (!window.confirm('确定立即执行每日重置吗？将清空全部活动的发布内容与领取记录以及幸运码，用户积分等数据不受影响')) return
  resetting.value = true
  try {
    await adminApi.dailyReset()
    emit('message', '已执行每日重置')
    refreshNow()
  } catch (error) {
    if (isAdminUnauthorized(error)) {
      emit('sessionExpired')
      return
    }
    loadError.value = friendlyAdminError(error)
  } finally {
    resetting.value = false
  }
}

onMounted(() => {
  loadQueues()
  refreshTimer = window.setInterval(() => {
    if (document.hidden) return
    countdown.value -= 1
    if (countdown.value <= 0) refreshNow()
  }, 1000)
})

onBeforeUnmount(() => window.clearInterval(refreshTimer))
</script>

<template>
  <div class="admin-panel-stack">
    <section class="admin-surface admin-queue-panel" :aria-busy="loading">
      <header class="admin-section-heading admin-queue-heading">
        <h2>排队队列状态</h2>
        <button class="admin-queue-reset" type="button" :disabled="resetting" @click="resetDaily">{{ resetting ? '正在重置…' : '重置' }}</button>
        <button class="admin-queue-refresh" type="button" aria-label="立即刷新" :disabled="loading" @click="refreshNow">
          <span class="admin-queue-refresh-dot" aria-hidden="true" />{{ countdown }}s后刷新
        </button>
      </header>

      <div v-if="loadError" class="admin-empty-state admin-empty-state--error" role="alert">
        <p>{{ loadError }}</p>
        <button type="button" @click="loadQueues">重新加载</button>
      </div>

      <div v-else class="admin-queue-list">
        <article v-for="item in queues" :key="item.type" class="admin-queue-activity">
          <header class="admin-queue-activity-head">
            <Activity :size="15" aria-hidden="true" />
            <h3>{{ activityConfigs[item.type].title }}</h3>
          </header>
          <div class="admin-queue-rows">
            <div v-for="lane in lanes" :key="lane.key" class="admin-queue-row">
              <span>{{ lane.label }}</span>
              <b v-if="statusOf(item, lane).created && lane.key === 'ordinary'">{{ statusOf(item, lane).position }}<small> / {{ statusOf(item, lane).total }}</small></b>
              <b v-else-if="statusOf(item, lane).created">{{ statusOf(item, lane).total }}</b>
              <span v-else class="admin-queue-row-empty">未创建</span>
            </div>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>
