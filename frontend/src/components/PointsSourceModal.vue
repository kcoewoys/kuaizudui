<script setup lang="ts">
import { ref, watch } from 'vue'
import BaseModal from './BaseModal.vue'
import { api, friendlyApiError, type PointRecord } from '@/services/api'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const records = ref<PointRecord[]>([])
const loading = ref(false)
const error = ref('')

const sourceNames: Record<string, string> = {
  exchange_code: '兑换码',
  admin_recharge: '运营充值',
  activity_boost: '积分加速',
  invite: '邀请好友',
}

async function loadHistory() {
  loading.value = true
  error.value = ''
  try {
    records.value = (await api.points.history(50)).items
  } catch (loadError) {
    error.value = friendlyApiError(loadError)
  } finally {
    loading.value = false
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

watch(() => props.open, (open) => {
  if (open) void loadHistory()
})
</script>

<template>
  <BaseModal :open="open" size="wide" @close="emit('close')">
    <template #title>积分来源</template>
    <template #subtitle>积分通过邀请，兑换码获得</template>
    <div class="points-table" role="table" aria-label="积分来源">
      <div class="points-row points-row--head" role="row">
        <span role="columnheader">来源</span>
        <span role="columnheader">说明</span>
        <span role="columnheader">时间</span>
        <span role="columnheader">积分</span>
      </div>
      <div v-for="item in records" :key="item.id" class="points-row" role="row">
        <strong role="cell">{{ sourceNames[item.source] || item.source }}</strong>
        <span role="cell">{{ item.description }}</span>
        <span role="cell">{{ formatTime(item.created_at) }}</span>
        <b role="cell">{{ item.points > 0 ? `+${item.points}` : item.points }}</b>
      </div>
    </div>
    <div v-if="loading" class="modal-list-state">正在加载积分记录…</div>
    <div v-else-if="error" class="modal-list-state modal-list-state--error">{{ error }}</div>
    <div v-else-if="records.length === 0" class="modal-list-state">暂无积分记录</div>
  </BaseModal>
</template>
