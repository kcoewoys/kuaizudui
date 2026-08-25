<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { MessageSquareText, RefreshCw } from 'lucide-vue-next'
import { adminApi, friendlyAdminError, isAdminUnauthorized, type AdminFeedbackRecord } from '@/services/adminApi'

const emit = defineEmits<{
  sessionExpired: []
}>()

const feedback = ref<AdminFeedbackRecord[]>([])
const loading = ref(true)
const loadError = ref('')

async function loadFeedback() {
  loading.value = true
  loadError.value = ''
  try {
    feedback.value = (await adminApi.feedback(50)).items
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

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

onMounted(loadFeedback)
</script>

<template>
  <div class="admin-panel-stack">
    <section class="admin-surface admin-feedback-panel" :aria-busy="loading">
      <header class="admin-section-heading admin-feedback-heading">
        <div>
          <h2>用户反馈</h2>
          <p>显示最近 50 条反馈，优先显示已绑定的手机号码。</p>
        </div>
        <button class="admin-secondary-button" type="button" :disabled="loading" @click="loadFeedback">
          <RefreshCw :size="16" />{{ loading ? '加载中…' : '刷新' }}
        </button>
      </header>

      <div v-if="loading" class="admin-loading-list" aria-label="正在加载用户反馈">
        <div v-for="index in 5" :key="index" class="admin-skeleton-row admin-feedback-skeleton" />
      </div>
      <div v-else-if="loadError" class="admin-empty-state admin-empty-state--error" role="alert">
        <p>{{ loadError }}</p>
        <button type="button" @click="loadFeedback">重新加载</button>
      </div>
      <div v-else-if="feedback.length === 0" class="admin-empty-state">
        <MessageSquareText :size="28" aria-hidden="true" />
        <p>暂时还没有用户反馈</p>
      </div>
      <ol v-else class="admin-feedback-list">
        <li v-for="item in feedback" :key="item.id">
          <header>
            <div class="admin-feedback-identity">
              <small>{{ item.phone ? '手机号码' : '用户 ID' }}</small>
              <strong>{{ item.phone || item.uid }}</strong>
            </div>
          </header>
          <p class="admin-feedback-content">{{ item.content }}</p>
          <footer class="admin-feedback-meta">
            <time :datetime="item.created_at" :aria-label="`提交时间 ${formatTime(item.created_at)}`">
              {{ formatTime(item.created_at) }}
            </time>
          </footer>
        </li>
      </ol>
    </section>
  </div>
</template>
