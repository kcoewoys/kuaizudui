<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { WalletCards } from 'lucide-vue-next'
import { adminApi, friendlyAdminError, isAdminUnauthorized, type RechargeRecord } from '@/services/adminApi'

const emit = defineEmits<{
  sessionExpired: []
  message: [message: string]
}>()

const recharges = ref<RechargeRecord[]>([])
const rechargeLoading = ref(true)
const rechargePhone = ref('')
const rechargePoints = ref<number | null>(null)
const rechargeError = ref('')
const recharging = ref(false)

function handleError(error: unknown) {
  if (isAdminUnauthorized(error)) {
    emit('sessionExpired')
    return
  }
  rechargeError.value = friendlyAdminError(error)
}

async function loadRecharges() {
  rechargeLoading.value = true
  try {
    recharges.value = (await adminApi.recharges(8)).items
  } catch (error) {
    if (isAdminUnauthorized(error)) emit('sessionExpired')
  } finally {
    rechargeLoading.value = false
  }
}

async function recharge() {
  const phone = rechargePhone.value.replace(/\s/g, '')
  const points = Number(rechargePoints.value)
  if (!/^1\d{10}$/.test(phone)) {
    rechargeError.value = '请输入已绑定的 11 位手机号码'
    return
  }
  if (!Number.isInteger(points) || points < 1 || points > 1_000_000) {
    rechargeError.value = '充值积分必须是 1 至 1,000,000 的整数'
    return
  }
  recharging.value = true
  rechargeError.value = ''
  try {
    const user = await adminApi.recharge(phone, points)
    rechargePhone.value = ''
    rechargePoints.value = null
    emit('message', `充值成功，当前积分 ${user.points}`)
    await loadRecharges()
  } catch (error) {
    handleError(error)
  } finally {
    recharging.value = false
  }
}

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

onMounted(loadRecharges)
</script>

<template>
  <div class="admin-panel-stack">
    <section class="admin-surface admin-action-panel">
      <h2>积分充值</h2>
      <p>输入已绑定的手机号码和积分数量，确认后立即到账。</p>
      <form @submit.prevent="recharge">
        <label for="recharge-phone">用户手机号码</label>
        <input id="recharge-phone" v-model="rechargePhone" type="tel" inputmode="numeric" maxlength="11" placeholder="例如 13800000001" @input="rechargeError = ''" />
        <label for="recharge-points">充值积分</label>
        <input id="recharge-points" v-model.number="rechargePoints" type="number" inputmode="numeric" min="1" max="1000000" placeholder="输入积分数量" @input="rechargeError = ''" />
        <p v-if="rechargeError" class="admin-form-error" role="alert">{{ rechargeError }}</p>
        <button class="admin-primary-button" type="submit" :disabled="recharging">{{ recharging ? '正在充值…' : '确认充值' }}</button>
      </form>
    </section>

    <section class="admin-surface admin-record-panel">
      <header class="admin-section-heading admin-section-heading--compact">
        <div>
          <h2>最近充值</h2>
          <p>显示最近 8 条操作记录。</p>
        </div>
        <WalletCards :size="20" aria-hidden="true" />
      </header>
      <div v-if="rechargeLoading" class="admin-loading-list"><div v-for="index in 3" :key="index" class="admin-skeleton-row" /></div>
      <p v-else-if="recharges.length === 0" class="admin-inline-empty">暂无充值记录</p>
      <ul v-else class="admin-record-list">
        <li v-for="record in recharges" :key="record.id">
          <div><strong>{{ record.phone }}</strong><span>{{ formatTime(record.created_at) }}</span></div>
          <b>+{{ record.points.toLocaleString('zh-CN') }}</b>
        </li>
      </ul>
    </section>
  </div>
</template>
