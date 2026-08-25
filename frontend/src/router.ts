import { createRouter, createWebHistory } from 'vue-router'
import ActivityListPage from '@/pages/ActivityListPage.vue'
import InviteActivityPage from '@/pages/InviteActivityPage.vue'
import LuckyTeamPage from '@/pages/LuckyTeamPage.vue'
import ProfilePage from '@/pages/ProfilePage.vue'
import { activityConfigs, type InviteActivityType } from '@/domain/activities'

const AdminPage = () => import('@/pages/AdminPage.vue')

const legacyHashPath = window.location.hash.startsWith('#/') ? window.location.hash.slice(1) : ''
if (legacyHashPath) {
  const basePath = import.meta.env.BASE_URL.replace(/\/$/, '')
  window.history.replaceState(window.history.state, '', `${basePath}${legacyHashPath}`)
}

const inviteRoute = (path: string, type: InviteActivityType) => ({
  path,
  component: InviteActivityPage,
  props: { activity: activityConfigs[type] },
})

export default createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', component: ActivityListPage },
    { path: '/admin', component: AdminPage },
    { path: '/profile', component: ProfilePage },
    { path: '/lucky-team', component: LuckyTeamPage },
    inviteRoute('/grocery-invite', 'buy_food'),
    inviteRoute('/cash-turntable', 'cash_turntable'),
    inviteRoute('/cash-monopoly', 'cash_monopoly'),
    inviteRoute('/daily-cash', 'daily_cash'),
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})
