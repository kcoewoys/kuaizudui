<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { CheckCircle2, Copy, RefreshCw } from 'lucide-vue-next'
import { adminApi, friendlyAdminError, isAdminUnauthorized, type ExchangeCodeRecord } from '@/services/adminApi'
import { copyText } from '@/utils/clipboard'

const emit = defineEmits<{
  sessionExpired: []
  message: [message: string]
}>()

const points = ref<number | null>(null)
const count = ref<number | null>(null)
const prefix = ref('')
const creating = ref(false)
const createError = ref('')
const createdCodes = ref<ExchangeCodeRecord[]>([])
const codes = ref<ExchangeCodeRecord[]>([])
const status = ref('')
const loading = ref(true)
const listError = ref('')

function handleError(error: unknown, target: { value: string }) {
  if (isAdminUnauthorized(error)) {
    emit('sessionExpired')
    return
  }
  target.value = friendlyAdminError(error)
}

async function loadCodes() {
  loading.value = true
  listError.value = ''
  try {
    codes.value = (await adminApi.exchangeCodes(status.value, 50)).items
  } catch (error) {
    handleError(error, listError)
  } finally {
    loading.value = false
  }
}

async function createCodes() {
  const pointsValue = Number(points.value)
  const countValue = Number(count.value)
  if (!Number.isInteger(pointsValue) || pointsValue < 1 || pointsValue > 1_000_000) {
    createError.value = '单个兑换码积分必须是 1 至 1,000,000 的整数'
    return
  }
  if (!Number.isInteger(countValue) || countValue < 1 || countValue > 500) {
    createError.value = '单次生成数量必须是 1 至 500 的整数'
    return
  }
  if (prefix.value.trim().length > 12) {
    createError.value = '兑换码前缀不能超过 12 个字符'
    return
  }

  creating.value = true
  createError.value = ''
  try {
    const result = await adminApi.createExchangeCodes(pointsValue, countValue, prefix.value.trim())
    createdCodes.value = result.items
    emit('message', `已生成 ${result.count} 个兑换码`)
    await loadCodes()
  } catch (error) {
    handleError(error, createError)
  } finally {
    creating.value = false
  }
}

async function copyCodes(items: ExchangeCodeRecord[]) {
  if (items.length === 0) return
  await copyText(items.map((item) => item.code).join('\n'))
  emit('message', `已复制 ${items.length} 个兑换码`)
}

function formatTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

onMounted(loadCodes)
</script>

<template>
  <div class="admin-panel-stack">
    <section class="admin-surface admin-generator">
      <div class="admin-generator-copy">
        <h2>批量生成兑换码</h2>
        <p>设置每个兑换码可获得的积分和生成数量。生成后可一次复制。</p>
      </div>
      <form class="admin-generator-form" @submit.prevent="createCodes">
        <div>
          <label for="exchange-points">每码积分</label>
          <input id="exchange-points" v-model.number="points" type="number" inputmode="numeric" min="1" max="1000000" placeholder="例如 20" @input="createError = ''" />
        </div>
        <div>
          <label for="exchange-count">生成数量</label>
          <input id="exchange-count" v-model.number="count" type="number" inputmode="numeric" min="1" max="500" placeholder="1–500" @input="createError = ''" />
        </div>
        <div>
          <label for="exchange-prefix">前缀（可选）</label>
          <input id="exchange-prefix" v-model="prefix" type="text" maxlength="12" placeholder="例如 VIP" @input="createError = ''" />
        </div>
        <button class="admin-primary-button" type="submit" :disabled="creating">{{ creating ? '正在生成…' : '生成兑换码' }}</button>
        <p v-if="createError" class="admin-form-error admin-generator-error" role="alert">{{ createError }}</p>
      </form>
    </section>

    <section v-if="createdCodes.length" class="admin-created-codes" aria-live="polite">
      <div>
        <CheckCircle2 :size="20" />
        <p><strong>本次生成 {{ createdCodes.length }} 个</strong><span>请及时复制或在下方记录中再次查看。</span></p>
      </div>
      <button type="button" @click="copyCodes(createdCodes)"><Copy :size="16" />复制本次兑换码</button>
    </section>

    <section class="admin-surface admin-code-directory">
      <header class="admin-section-heading admin-code-heading">
        <div>
          <h2>兑换码记录</h2>
          <p>显示最近 50 条兑换码及使用状态。</p>
        </div>
        <div class="admin-filter-actions">
          <label class="sr-only" for="exchange-status">兑换码状态</label>
          <select id="exchange-status" v-model="status" @change="loadCodes">
            <option value="">全部状态</option>
            <option value="unused">未使用</option>
            <option value="used">已使用</option>
          </select>
          <button class="admin-secondary-button" type="button" :disabled="loading" @click="loadCodes"><RefreshCw :size="16" />刷新</button>
          <button class="admin-secondary-button" type="button" :disabled="codes.length === 0" @click="copyCodes(codes)"><Copy :size="16" />复制当前列表</button>
        </div>
      </header>

      <div v-if="loading" class="admin-loading-list" aria-label="正在加载兑换码">
        <div v-for="index in 6" :key="index" class="admin-skeleton-row" />
      </div>
      <div v-else-if="listError" class="admin-empty-state admin-empty-state--error">
        <p>{{ listError }}</p><button type="button" @click="loadCodes">重新加载</button>
      </div>
      <p v-else-if="codes.length === 0" class="admin-inline-empty">当前筛选条件下没有兑换码</p>
      <div v-else class="admin-data-table admin-code-table" role="table" aria-label="兑换码列表">
        <div class="admin-table-head" role="row">
          <span role="columnheader">兑换码</span><span role="columnheader">积分</span><span role="columnheader">状态</span><span role="columnheader">生成时间</span>
        </div>
        <div v-for="item in codes" :key="item.id" class="admin-table-row" role="row">
          <span class="admin-code-value" data-label="兑换码" role="cell">
            <code>{{ item.code }}</code>
          </span>
          <strong class="admin-code-points" data-label="积分" role="cell">{{ item.points.toLocaleString('zh-CN') }}</strong>
          <span class="admin-code-status" data-label="状态" role="cell"><b class="admin-status" :class="`admin-status--${item.status}`">{{ item.status === 'unused' ? '未使用' : '已使用' }}</b></span>
          <time class="admin-code-created" :datetime="item.created_at" :aria-label="`生成时间：${formatTime(item.created_at)}`" role="cell">{{ formatTime(item.created_at) }}</time>
        </div>
      </div>
    </section>
  </div>
</template>
