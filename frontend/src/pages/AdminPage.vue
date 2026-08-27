<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Activity, ArrowLeft, Coins, LogOut, MessageSquareText, Settings2, ShieldCheck, TicketCheck } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import AdminConfigPanel from '@/components/admin/AdminConfigPanel.vue'
import AdminExchangePanel from '@/components/admin/AdminExchangePanel.vue'
import AdminFeedbackPanel from '@/components/admin/AdminFeedbackPanel.vue'
import AdminPhoneGate from '@/components/admin/AdminPhoneGate.vue'
import AdminRechargePanel from '@/components/admin/AdminRechargePanel.vue'
import AdminStatusPanel from '@/components/admin/AdminStatusPanel.vue'
import ToastMessage from '@/components/ToastMessage.vue'
import { adminApi, friendlyAdminError } from '@/services/adminApi'
import '@/admin.css'

type AdminSection = 'recharge' | 'exchanges' | 'config' | 'feedback' | 'status'

const router = useRouter()
const verified = ref(false)
const verifying = ref(false)
const gateError = ref('')
const activeSection = ref<AdminSection>('recharge')
const toast = ref('')
let toastTimer: number | undefined

const navigation = [
  { id: 'recharge' as const, label: '积分充值', description: '充值与操作记录', icon: Coins },
  { id: 'exchanges' as const, label: '兑换码', description: '生成与使用状态', icon: TicketCheck },
  { id: 'config' as const, label: '内容配置', description: '公告与交流群', icon: Settings2 },
  { id: 'feedback' as const, label: '反馈', description: '用户意见与提交时间', icon: MessageSquareText },
  { id: 'status' as const, label: '状态', description: '排队队列游标监视', icon: Activity },
]

const activeItem = computed(() => navigation.find((item) => item.id === activeSection.value) ?? navigation[0])

function showToast(message: string) {
  toast.value = message
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (toast.value = ''), 2400)
}

async function verifyPhone(phone: string) {
  verifying.value = true
  gateError.value = ''
  try {
    await adminApi.verifyPhone(phone)
    verified.value = true
    showToast('管理员身份验证成功')
  } catch (error) {
    gateError.value = friendlyAdminError(error)
  } finally {
    verifying.value = false
  }
}

function sessionExpired() {
  adminApi.clearSession()
  verified.value = false
  gateError.value = '管理员验证已失效，请重新输入手机号码'
}

async function leaveAdmin() {
  await adminApi.logout()
  verified.value = false
  router.push('/')
}

async function exitSession() {
  await adminApi.logout()
  verified.value = false
  gateError.value = ''
}

onMounted(() => adminApi.clearSession())
</script>

<template>
  <main class="admin-shell">
    <AdminPhoneGate :open="!verified" :busy="verifying" :error="gateError" @verify="verifyPhone" @clear-error="gateError = ''" />

    <template v-if="verified">
      <div class="admin-mobile-frame">
        <header class="admin-appbar">
          <button class="admin-appbar-action admin-appbar-action--back" type="button" aria-label="转到客户端" @click="leaveAdmin"><ArrowLeft :size="17" />转到客户端</button>

          <div class="admin-brand">
            <span><ShieldCheck :size="21" /></span>
            <div><strong>eaok.cn</strong><small>运营管理台</small></div>
          </div>

          <button class="admin-appbar-action admin-appbar-action--exit" type="button" aria-label="退出管理" @click="exitSession"><LogOut :size="17" />退出</button>
        </header>

        <section class="admin-workspace">
          <header v-if="activeSection !== 'status'" class="admin-topbar">
            <div>
              <h1>{{ activeItem.label }}</h1>
              <p>{{ activeItem.description }}</p>
            </div>
          </header>

          <div class="admin-workspace-content" :class="{ 'admin-workspace-content--flush': activeSection === 'status' }">
            <AdminStatusPanel v-if="activeSection === 'status'" @session-expired="sessionExpired" />
            <AdminRechargePanel v-else-if="activeSection === 'recharge'" @session-expired="sessionExpired" @message="showToast" />
            <AdminExchangePanel v-else-if="activeSection === 'exchanges'" @session-expired="sessionExpired" @message="showToast" />
            <AdminConfigPanel v-else-if="activeSection === 'config'" @session-expired="sessionExpired" @message="showToast" />
            <AdminFeedbackPanel v-else @session-expired="sessionExpired" />
          </div>
        </section>

        <nav class="admin-nav" aria-label="管理员功能">
          <button
            v-for="item in navigation"
            :key="item.id"
            type="button"
            :aria-current="activeSection === item.id ? 'page' : undefined"
            :class="{ 'admin-nav-item--active': activeSection === item.id }"
            @click="activeSection = item.id"
          >
            <component :is="item.icon" :size="20" />
            <strong>{{ item.label }}</strong>
          </button>
        </nav>
      </div>
    </template>

    <ToastMessage :message="toast" />
  </main>
</template>
