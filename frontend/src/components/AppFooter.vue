<script setup lang="ts">
import { computed, ref } from 'vue'
import BaseModal from './BaseModal.vue'
import { api, friendlyApiError } from '@/services/api'

const openPanel = ref<'about' | 'group' | 'feedback' | null>(null)
const qrcode = ref('')
const qrcodeMessage = ref('')
const feedbackContent = ref('')
const feedbackMessage = ref('')
const feedbackState = ref<'idle' | 'success' | 'error'>('idle')
const feedbackSubmitting = ref(false)
const feedbackLength = computed(() => Array.from(feedbackContent.value).length)

function openFeedback() {
  openPanel.value = 'feedback'
  feedbackMessage.value = ''
  feedbackState.value = 'idle'
}

function clearFeedbackMessage() {
  feedbackMessage.value = ''
  feedbackState.value = 'idle'
}

async function submitFeedback() {
  const content = feedbackContent.value.trim()
  if (!content) {
    feedbackMessage.value = '请填写反馈内容'
    feedbackState.value = 'error'
    return
  }
  if (Array.from(content).length > 500) {
    feedbackMessage.value = '反馈内容不能超过 500 字'
    feedbackState.value = 'error'
    return
  }

  feedbackSubmitting.value = true
  feedbackMessage.value = ''
  feedbackState.value = 'idle'
  try {
    await api.public.submitFeedback(content)
    feedbackContent.value = ''
    feedbackMessage.value = '感谢反馈，我们已经收到。'
    feedbackState.value = 'success'
  } catch (error) {
    feedbackMessage.value = friendlyApiError(error)
    feedbackState.value = 'error'
  } finally {
    feedbackSubmitting.value = false
  }
}

async function openGroup() {
  openPanel.value = 'group'
  qrcodeMessage.value = '正在加载二维码…'
  try {
    qrcode.value = (await api.public.groupQRCode()).qrcode
    qrcodeMessage.value = qrcode.value ? '长按识别后加入交流群。' : '管理员暂未配置交流群二维码。'
  } catch (error) {
    qrcodeMessage.value = friendlyApiError(error)
  }
}
</script>

<template>
  <footer class="app-footer" aria-label="辅助导航">
    <button type="button" @click="openPanel = 'about'">关于</button>
    <button type="button" @click="openGroup">交流群</button>
    <button type="button" @click="openFeedback">反馈</button>
  </footer>

  <BaseModal :open="openPanel !== null" size="small" @close="openPanel = null">
    <template #title>
      {{ openPanel === 'about' ? '关于 eaok.cn' : openPanel === 'group' ? '加入交流群' : '意见反馈' }}
    </template>
    <div v-if="openPanel === 'about'" class="simple-panel">
      <p>一个专注于福袋组队与现金活动分享的移动端工具。</p>
      <p>请诚信发布有效内容，共同维护顺畅的领取体验。</p>
    </div>
    <div v-else-if="openPanel === 'group'" class="simple-panel simple-panel--centered">
      <img v-if="qrcode" class="group-qrcode" :src="qrcode" alt="交流群二维码" />
      <div v-else class="qr-placeholder" aria-label="交流群二维码占位">
        <span>群二维码</span>
      </div>
      <p>{{ qrcodeMessage }}</p>
    </div>
    <form v-else class="feedback-form" :aria-busy="feedbackSubmitting" @submit.prevent="submitFeedback">
      <p class="feedback-intro">遇到问题或有产品建议，都可以直接告诉我们。</p>
      <label for="feedback-content">反馈内容</label>
      <textarea
        id="feedback-content"
        v-model="feedbackContent"
        class="feedback-textarea"
        maxlength="500"
        placeholder="请描述遇到的问题或你的建议"
        :disabled="feedbackSubmitting"
        :aria-invalid="feedbackState === 'error'"
        aria-describedby="feedback-meta"
        @input="clearFeedbackMessage"
      />
      <div id="feedback-meta" class="feedback-meta">
        <span
          class="feedback-message"
          :class="`feedback-message--${feedbackState}`"
          :role="feedbackState === 'error' ? 'alert' : feedbackMessage ? 'status' : undefined"
          :aria-live="feedbackState === 'error' ? 'assertive' : 'polite'"
        >{{ feedbackMessage }}</span>
        <span>{{ feedbackLength }}/500</span>
      </div>
      <button class="primary-button" type="submit" :disabled="feedbackSubmitting">
        {{ feedbackSubmitting ? '正在提交…' : '确定提交' }}
      </button>
    </form>
  </BaseModal>
</template>
