<script setup lang="ts">
import { ref } from 'vue'
import { ArrowLeft, Bolt, Clock3, HelpCircle, Minus, Plus, TrendingUp, UserRound, UsersRound } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import type { InviteActivityConfig } from '@/domain/activities'
import { useInviteActivity } from '@/composables/useInviteActivity'
import MobileShell from '@/components/MobileShell.vue'
import AppFooter from '@/components/AppFooter.vue'
import GuideModal from '@/components/GuideModal.vue'
import ToastMessage from '@/components/ToastMessage.vue'

const props = defineProps<{ activity: InviteActivityConfig }>()
const router = useRouter()
const guideOpen = ref(false)

const activityState = useInviteActivity(props.activity)
const {
  state,
  draft,
  notice,
  error,
  toast,
  remaining,
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
} = activityState

</script>

<template>
  <MobileShell>
    <header class="page-header">
      <button class="icon-button" type="button" aria-label="返回" @click="router.push('/')">
        <ArrowLeft :size="21" />
      </button>
      <h1>{{ activity.title }}</h1>
      <button class="profile-button profile-button--soft" type="button" aria-label="个人中心" @click="router.push('/profile')">
        <UserRound :size="18" />
      </button>
    </header>

    <section class="info-panel">
      <p>{{ notice }}</p>
    </section>

    <button class="guide-button" type="button" @click="guideOpen = true">
      <HelpCircle :size="17" />
      <span>{{ activity.guideTitle }}</span>
    </button>

    <section class="publish-panel card-surface">
      <label class="sr-only" :for="`${activity.type}-content`">邀请内容</label>
      <textarea
        :id="`${activity.type}-content`"
        v-model="draft"
        class="publish-input"
        :class="{ 'publish-input--error': error }"
        :placeholder="activity.placeholder"
        maxlength="201"
        :disabled="state.loading || state.working"
      />
      <div class="publish-actions">
        <span :class="{ 'counter--error': remaining < 0 }">{{ draft.length }}/200</span>
        <button class="primary-button publish-button" :disabled="state.loading || state.working" type="button" @click="activityState.publish">
          {{ state.working ? '处理中' : '立即发布' }}
        </button>
      </div>
      <p v-if="error" class="field-error publish-error">{{ error }}</p>
    </section>

    <section v-if="state.loading" class="empty-published card-surface loading-card" aria-label="正在加载活动数据">
      <div class="skeleton-line skeleton-line--short" />
      <div class="skeleton-line" />
      <div class="skeleton-line" />
    </section>

    <section v-else-if="isPublished" class="my-invite card-surface">
      <div class="invite-heading">
        <div>
          <h2>{{ maskedContent }}</h2>
          <p><Clock3 :size="13" /> {{ recordTimestampLabel }} {{ new Date(recordTimestamp).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) }}</p>
        </div>
        <span class="status-chip"><span /> 排队进行中</span>
      </div>

      <div class="stat-grid">
        <div class="stat-block stat-block--blue">
          <UsersRound :size="17" />
          <span>普通轮数</span>
          <strong aria-live="polite" aria-atomic="true">{{ state.ordinaryRounds }} <small>轮 / {{ state.claimCount }} 次领码</small></strong>
        </div>
        <div class="stat-block stat-block--mint">
          <Bolt :size="17" />
          <span>插队轮数</span>
          <strong aria-live="polite" aria-atomic="true">{{ state.priorityRounds }} <small>轮 / {{ state.pointsCommitted }} 积分</small></strong>
        </div>
      </div>

      <div class="boost-row">
        <div class="boost-summary">
          <span class="boost-icon" aria-hidden="true"><TrendingUp :size="19" /></span>
          <strong>使用积分插队</strong>
          <small>可用 <b>{{ userPoints }}</b> 积分</small>
        </div>
        <div class="boost-actions">
          <div class="stepper" aria-label="调整使用积分数">
            <button type="button" aria-label="减少积分" :disabled="!canDecreaseBoostPoints" @click="activityState.adjustBoostPoints(-1)"><Minus :size="20" /></button>
            <input
              v-model="boostPointsInput"
              aria-label="使用积分数"
              type="number"
              inputmode="numeric"
              min="1"
              :max="Math.max(1, maxBoostPoints)"
              step="1"
              :disabled="state.working || maxBoostPoints < 1"
              @blur="activityState.normalizeBoostPoints"
              @keydown.enter.prevent="activityState.useBoost"
            />
            <button type="button" aria-label="增加积分" :disabled="!canIncreaseBoostPoints" @click="activityState.adjustBoostPoints(1)"><Plus :size="20" /></button>
          </div>
          <button class="primary-button boost-button" :disabled="!canUseBoost" type="button" @click="activityState.useBoost">确认使用</button>
        </div>
      </div>
    </section>

    <section v-else class="empty-published card-surface">
      <div class="empty-ring"><Clock3 :size="24" /></div>
      <h2>还没有发布内容</h2>
      <p>发布后会在这里显示你的排队与加速状态。</p>
    </section>

    <button class="primary-button receive-button" type="button" :disabled="!isPublished || !state.canClaim || state.working" @click="activityState.copyContent">
      一键领码
    </button>

    <AppFooter />
    <GuideModal :open="guideOpen" :title="activity.guideTitle" :steps="activity.guideSteps" @close="guideOpen = false" />
    <ToastMessage :message="toast" />
  </MobileShell>
</template>
