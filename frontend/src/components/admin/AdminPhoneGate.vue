<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { ShieldCheck, Smartphone } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
  busy: boolean
  error: string
}>()

const emit = defineEmits<{
  verify: [phone: string]
  clearError: []
}>()

const phone = ref('')
const localError = ref('')
const phoneInput = ref<HTMLInputElement | null>(null)

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    phone.value = ''
    localError.value = ''
    await nextTick()
    phoneInput.value?.focus()
  },
  { immediate: true },
)

function submit() {
  const value = phone.value.replace(/\s/g, '')
  if (!/^1\d{10}$/.test(value)) {
    localError.value = '请输入正确的 11 位手机号码'
    return
  }
  emit('verify', value)
}

function changed() {
  localError.value = ''
  emit('clearError')
}
</script>

<template>
  <Teleport to="body">
    <Transition name="admin-gate">
      <div v-if="open" class="admin-gate-backdrop">
        <section class="admin-gate" role="dialog" aria-modal="true" aria-labelledby="admin-gate-title" aria-describedby="admin-gate-description">
          <div class="admin-gate-mark" aria-hidden="true"><ShieldCheck :size="29" :stroke-width="2" /></div>
          <h1 id="admin-gate-title">验证管理员手机号</h1>
          <p id="admin-gate-description">管理后台不设置密码。输入后端配置的管理员手机号，通过验证后即可进入。</p>

          <form class="admin-gate-form" @submit.prevent="submit">
            <label for="admin-phone">手机号码</label>
            <div class="admin-gate-input-wrap">
              <Smartphone :size="18" aria-hidden="true" />
              <input
                id="admin-phone"
                ref="phoneInput"
                v-model="phone"
                type="tel"
                inputmode="numeric"
                autocomplete="tel"
                maxlength="11"
                placeholder="请输入 11 位手机号码"
                :disabled="busy"
                @input="changed"
              />
            </div>
            <p v-if="localError || error" class="admin-form-error" role="alert">{{ localError || error }}</p>
            <p v-else class="admin-gate-helper">号码仅发送到服务器校验，正确号码不会写入前端代码。</p>
            <button class="admin-primary-button" type="submit" :disabled="busy">
              {{ busy ? '正在验证…' : '进入管理后台' }}
            </button>
          </form>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>
