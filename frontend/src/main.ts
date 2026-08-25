import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './style.css'
import { captureReferralFromUrl } from './services/session'

function syncAppViewportHeight() {
  const viewportHeight = Math.max(window.innerHeight, document.documentElement.clientHeight)

  if (viewportHeight > 0) {
    document.documentElement.style.setProperty('--app-viewport-height', `${Math.round(viewportHeight)}px`)
  }
}

syncAppViewportHeight()
window.addEventListener('resize', syncAppViewportHeight, { passive: true })
window.visualViewport?.addEventListener('resize', syncAppViewportHeight, { passive: true })

void captureReferralFromUrl()

createApp(App).use(router).mount('#app')
