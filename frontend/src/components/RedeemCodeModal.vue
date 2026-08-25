<script setup lang="ts">
import { ref } from 'vue'
import { ArrowRight, Ticket } from 'lucide-vue-next'
import BaseModal from './BaseModal.vue'
import { api, friendlyApiError } from '@/services/api'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; redeemed: [points: number, total: number] }>()

const code = ref('')
const error = ref('')
const loading = ref(false)

async function redeem() {
  const value = code.value.trim().toUpperCase()
  if (!value) {
    error.value = '请输入兑换码'
    return
  }
  loading.value = true
  try {
    const result = await api.points.exchange(value)
    emit('redeemed', result.awarded_points, result.total_points)
    code.value = ''
    error.value = ''
  } catch (redeemError) {
    error.value = friendlyApiError(redeemError)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <BaseModal :open="open" size="normal" @close="emit('close')">
    <template #title>兑换码兑换</template>
    <div class="redeem-hero">
      <div class="redeem-icon"><Ticket :size="26" /></div>
      <div>
        <span>兑换码来源</span>
        <strong>交流群 · 限时活动</strong>
      </div>
    </div>
    <label class="field-label" for="redeem-code">兑换码</label>
    <input
      id="redeem-code"
      v-model="code"
      class="text-input code-input"
      maxlength="24"
      autocomplete="off"
      placeholder="请输入兑换码"
      @input="error = ''"
      @keyup.enter="redeem"
    />
    <p v-if="error" class="field-error">{{ error }}</p>
    <p v-else class="field-helper">兑换成功后，积分会立即进入你的账户。</p>
    <button class="primary-button full-width" :disabled="loading" type="button" @click="redeem">
      <span>{{ loading ? '正在验证' : '确认兑换' }}</span>
      <ArrowRight v-if="!loading" :size="18" />
    </button>
  </BaseModal>
</template>
