<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ChevronRight, UserRound, X } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import MobileShell from '@/components/MobileShell.vue'
import AppFooter from '@/components/AppFooter.vue'
import ActivityMark from '@/components/ActivityMark.vue'
import { inviteActivities } from '@/domain/activities'
import { api } from '@/services/api'
import { loadUser } from '@/services/session'

const router = useRouter()
const showNotice = ref(true)
const noticeText = ref('')

onMounted(async () => {
  await Promise.allSettled([
    loadUser(),
    api.public.notice('home').then((notice) => {
      noticeText.value = notice.content
      showNotice.value = Boolean(notice.content)
    }),
  ])
})
</script>

<template>
  <MobileShell class="activity-home">
    <header class="home-header">
      <div>
        <p class="eyebrow">组队邀请平台</p>
        <h1>eaok.cn</h1>
      </div>
      <button class="profile-button" type="button" aria-label="个人中心" @click="router.push('/profile')">
        <UserRound :size="18" :stroke-width="2.2" />
      </button>
    </header>

    <Transition name="notice">
      <div v-if="showNotice && noticeText" class="announcement">
        <span class="announcement-dot" />
        <p>公告：{{ noticeText }}</p>
        <button type="button" aria-label="关闭公告" @click="showNotice = false"><X :size="16" /></button>
      </div>
    </Transition>

    <nav class="activity-stack" aria-label="活动列表">
      <button class="activity-card activity-card--featured" type="button" @click="router.push('/lucky-team')">
        <ActivityMark icon="lucky" />
        <span class="activity-copy">
          <strong>福袋组队</strong>
          <small>快速匹配可用福袋码</small>
        </span>
        <ChevronRight class="activity-chevron" :size="20" />
      </button>

      <button
        v-for="(item, index) in inviteActivities"
        :key="item.type"
        class="activity-card"
        :style="{ '--delay': `${(index + 1) * 50}ms` }"
        type="button"
        @click="router.push(item.path)"
      >
        <ActivityMark :icon="item.icon" />
        <span class="activity-copy"><strong>{{ item.title }}</strong></span>
        <ChevronRight class="activity-chevron" :size="20" />
      </button>
    </nav>

    <AppFooter />
  </MobileShell>
</template>
