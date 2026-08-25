<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowLeft, BarChart3, ChevronRight, Copy, Link2, Smartphone } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import MobileShell from '@/components/MobileShell.vue'
import AppFooter from '@/components/AppFooter.vue'
import BaseModal from '@/components/BaseModal.vue'
import PointsSourceModal from '@/components/PointsSourceModal.vue'
import RedeemCodeModal from '@/components/RedeemCodeModal.vue'
import ToastMessage from '@/components/ToastMessage.vue'
import { activityConfigs, type InviteActivityType } from '@/domain/activities'
import { api, friendlyApiError, type ActivityStateResponse, type LuckyStats } from '@/services/api'
import { applyPoints, bindUserPhone, captureReferralFromUrl, loadUser, userState } from '@/services/session'
import { copyText } from '@/utils/clipboard'

interface ProfileActivityStat {
  type: InviteActivityType
  title: string
  path: string
  state: ActivityStateResponse
}

const router = useRouter()
const pointsOpen = ref(false)
const redeemOpen = ref(false)
const phoneOpen = ref(false)
const phoneDraft = ref('')
const phoneError = ref('')
const toast = ref('')
const loading = ref(true)
const luckyStats = ref<LuckyStats>({ claimed_today: 0, published_today: 0 })
const stats = ref<ProfileActivityStat[]>([])

const activityTypes = Object.keys(activityConfigs) as InviteActivityType[]
const maskedUID = computed(() => userState.uid ? userState.uid.slice(0, 8).toUpperCase() : '加载中')
const inviteUrl = computed(() => {
  if (!userState.phone) return ''
  const url = new URL(import.meta.env.BASE_URL, window.location.origin)
  url.searchParams.set('ref', userState.phone)
  return url.toString()
})

function showToast(message: string) {
  toast.value = message
  window.setTimeout(() => (toast.value = ''), 2200)
}

async function loadProfile() {
  loading.value = true
  try {
    await captureReferralFromUrl()
    await loadUser(true)
    phoneDraft.value = userState.phone
    const [activities, lucky] = await Promise.all([
      Promise.all(activityTypes.map((type) => api.activity.detail(type))),
      api.lucky.stats(),
    ])
    stats.value = activities.map((state) => ({
      type: state.type,
      title: activityConfigs[state.type].title,
      path: activityConfigs[state.type].path,
      state,
    }))
    luckyStats.value = lucky
  } catch (loadError) {
    showToast(friendlyApiError(loadError))
  } finally {
    loading.value = false
  }
}

async function bindPhone() {
  const value = phoneDraft.value.replace(/\s/g, '')
  if (!/^1\d{10}$/.test(value)) {
    phoneError.value = '请输入正确的 11 位手机号码'
    return
  }
  try {
    await bindUserPhone(value)
    phoneOpen.value = false
    showToast('手机号码已绑定')
  } catch (bindError) {
    phoneError.value = friendlyApiError(bindError)
  }
}

function openPhoneBinding() {
  phoneError.value = ''
  phoneDraft.value = userState.phone
  phoneOpen.value = true
}

async function copyInviteLink() {
  if (!inviteUrl.value) return
  await copyText(inviteUrl.value)
  showToast('邀请链接已复制')
}

function maskPhone(value: string) {
  return value ? `${value.slice(0, 3)}****${value.slice(-4)}` : '添加手机号码'
}

function redeemed(points: number, total: number) {
  applyPoints(total)
  redeemOpen.value = false
  showToast(`兑换成功，积分 +${points}`)
}

onMounted(loadProfile)
</script>

<template>
  <MobileShell>
    <header class="profile-header">
      <button class="icon-button" type="button" aria-label="返回" @click="router.push('/')"><ArrowLeft :size="21" /></button>
      <strong>ID: {{ maskedUID }}</strong>
      <button type="button" :disabled="Boolean(userState.phone)" @click="openPhoneBinding">{{ userState.phone || '添加手机号码' }}</button>
    </header>

    <section class="points-card">
      <span>积分</span>
      <div class="points-card-main">
        <strong>{{ loading ? '—' : userState.points }}</strong>
        <div>
          <button type="button" @click="pointsOpen = true">积分来源</button>
          <button type="button" @click="redeemOpen = true">有兑换码？<u>立即兑换</u></button>
        </div>
      </div>
    </section>

    <section class="referral-panel card-surface" aria-labelledby="referral-title">
      <header class="referral-heading">
        <span class="referral-icon" aria-hidden="true"><Link2 :size="18" /></span>
        <div>
          <h2 id="referral-title">邀请好友</h2>
          <p>好友绑定手机号码后会各自获得 10 积分</p>
        </div>
      </header>

      <template v-if="userState.phone">
        <label class="field-label" for="invite-link">专属邀请链接</label>
        <div class="referral-link-row">
          <input id="invite-link" class="text-input" :value="inviteUrl" readonly aria-label="专属邀请链接" />
          <button class="secondary-button" type="button" @click="copyInviteLink"><Copy :size="16" />复制</button>
        </div>
      </template>
      <template v-else>
        <p class="referral-empty">绑定手机号后才能生成邀请链接。</p>
        <button class="secondary-button referral-bind-button" type="button" @click="openPhoneBinding">绑定手机号</button>
      </template>
      <p class="referral-inviter-note">
        <template v-if="userState.invitedByPhone">由 <strong>{{ maskPhone(userState.invitedByPhone) }}</strong> 邀请</template>
        <template v-else>无人邀请</template>
      </p>
    </section>

    <section class="profile-stats">
      <h2><BarChart3 :size="19" /> 今日统计数据</h2>
      <div v-if="loading" class="profile-loading">
        <div v-for="index in 5" :key="index" class="skeleton-line" />
      </div>
      <template v-else>
        <button class="profile-stat" type="button" @click="router.push('/lucky-team')">
          <span>福袋组队</span><strong>{{ luckyStats.claimed_today }} / {{ luckyStats.published_today }}</strong><ChevronRight :size="18" />
        </button>
        <button v-for="item in stats" :key="item.type" class="profile-stat" type="button" @click="router.push(item.path)">
          <span>{{ item.title }}</span>
          <strong>{{ item.state.claim_count }} / {{ item.state.published ? 1 : 0 }}</strong>
          <ChevronRight :size="18" />
        </button>
      </template>
    </section>

    <AppFooter />
    <PointsSourceModal :open="pointsOpen" @close="pointsOpen = false" />
    <RedeemCodeModal :open="redeemOpen" @close="redeemOpen = false" @redeemed="redeemed" />
    <BaseModal :open="phoneOpen" size="small" @close="phoneOpen = false">
      <template #title>绑定手机号码</template>
      <div class="phone-icon"><Smartphone :size="24" /></div>
      <label class="field-label" for="phone">手机号码</label>
      <input id="phone" v-model="phoneDraft" class="text-input" inputmode="tel" maxlength="11" placeholder="请输入 11 位手机号码" @input="phoneError = ''" />
      <p v-if="phoneError" class="field-error">{{ phoneError }}</p>
      <p v-else class="field-helper">手机号绑定后不可自行更换，用于积分归属与账号识别。</p>
      <button class="primary-button full-width" type="button" @click="bindPhone">确认绑定</button>
    </BaseModal>
    <ToastMessage :message="toast" />
  </MobileShell>
</template>
