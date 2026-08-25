<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { FileUp, Image, Save, Trash2, X } from 'lucide-vue-next'
import { ApiError } from '@/services/api'
import { adminApi, friendlyAdminError, isAdminUnauthorized } from '@/services/adminApi'

const emit = defineEmits<{
  sessionExpired: []
  message: [message: string]
}>()

const noticeTypes = [
  { value: 'home', label: '首页公告' },
  { value: 'lucky', label: '福袋组队提示' },
  { value: 'buy_food', label: '买菜邀请提示' },
  { value: 'cash_turntable', label: '现金大转盘提示' },
  { value: 'cash_monopoly', label: '现金大富翁提示' },
  { value: 'daily_cash', label: '天天领现金提示' },
]

const noticeType = ref('home')
const noticeContent = ref('')
const noticeLoading = ref(true)
const noticeSaving = ref(false)
const noticeError = ref('')
const qrcode = ref('')
const qrcodeLoading = ref(true)
const qrcodeSaving = ref(false)
const qrcodeError = ref('')
const qrcodeMaxUploadBytes = ref(5 * 1024 * 1024)
const selectedQRCode = ref<File | null>(null)
const selectedQRCodePreview = ref('')
const qrcodeFileInput = ref<HTMLInputElement | null>(null)

const displayedQRCode = computed(() => selectedQRCodePreview.value || qrcode.value)
const qrcodeMaxUploadMB = computed(() => Math.max(1, Math.floor(qrcodeMaxUploadBytes.value / 1024 / 1024)))

function handleError(error: unknown, target: { value: string }) {
  if (isAdminUnauthorized(error)) {
    emit('sessionExpired')
    return
  }
  target.value = friendlyAdminError(error)
}

async function loadNotice() {
  noticeLoading.value = true
  noticeError.value = ''
  try {
    noticeContent.value = (await adminApi.notice(noticeType.value)).content
  } catch (error) {
    if (error instanceof ApiError && error.code === 'not_found') {
      noticeContent.value = ''
    } else {
      handleError(error, noticeError)
    }
  } finally {
    noticeLoading.value = false
  }
}

async function saveNotice() {
  noticeSaving.value = true
  noticeError.value = ''
  try {
    const result = await adminApi.setNotice(noticeType.value, noticeContent.value)
    noticeContent.value = result.content
    emit('message', '公告内容已保存')
  } catch (error) {
    handleError(error, noticeError)
  } finally {
    noticeSaving.value = false
  }
}

async function loadQRCode() {
  qrcodeLoading.value = true
  qrcodeError.value = ''
  try {
    const result = await adminApi.groupQRCode()
    qrcode.value = result.qrcode
    if (Number.isFinite(result.max_upload_bytes) && result.max_upload_bytes > 0) {
      qrcodeMaxUploadBytes.value = result.max_upload_bytes
    }
  } catch (error) {
    handleError(error, qrcodeError)
  } finally {
    qrcodeLoading.value = false
  }
}

function revokeQRCodePreview() {
  if (selectedQRCodePreview.value) URL.revokeObjectURL(selectedQRCodePreview.value)
  selectedQRCodePreview.value = ''
}

function clearSelectedQRCode(clearError = true) {
  revokeQRCodePreview()
  selectedQRCode.value = null
  if (qrcodeFileInput.value) qrcodeFileInput.value.value = ''
  if (clearError) qrcodeError.value = ''
}

function selectQRCode(event: Event) {
  const input = event.currentTarget as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const supported = file.type === 'image/png' || file.type === 'image/jpeg'
  if (!supported) {
    clearSelectedQRCode(false)
    qrcodeError.value = '请选择 PNG 或 JPG 格式的图片'
    return
  }
  if (file.size > qrcodeMaxUploadBytes.value) {
    clearSelectedQRCode(false)
    qrcodeError.value = `图片不能超过 ${qrcodeMaxUploadMB.value} MB`
    return
  }
  revokeQRCodePreview()
  selectedQRCode.value = file
  selectedQRCodePreview.value = URL.createObjectURL(file)
  qrcodeError.value = ''
}

async function uploadQRCode() {
  if (!selectedQRCode.value) {
    qrcodeError.value = '请先选择二维码图片'
    return
  }
  qrcodeSaving.value = true
  qrcodeError.value = ''
  try {
    qrcode.value = (await adminApi.uploadGroupQRCode(selectedQRCode.value)).qrcode
    clearSelectedQRCode(false)
    emit('message', '交流群二维码已更新')
  } catch (error) {
    if (error instanceof ApiError && error.code === 'invalid_input') {
      qrcodeError.value = '图片文件无效，请选择清晰的 PNG 或 JPG 图片'
    } else {
      handleError(error, qrcodeError)
    }
  } finally {
    qrcodeSaving.value = false
  }
}

async function removeQRCode() {
  if (!window.confirm('确定移除当前交流群二维码吗？')) return
  qrcodeSaving.value = true
  qrcodeError.value = ''
  try {
    await adminApi.removeGroupQRCode()
    qrcode.value = ''
    emit('message', '交流群二维码已移除')
  } catch (error) {
    handleError(error, qrcodeError)
  } finally {
    qrcodeSaving.value = false
  }
}

onMounted(() => Promise.all([loadNotice(), loadQRCode()]))
onBeforeUnmount(revokeQRCodePreview)
</script>

<template>
  <div class="admin-config-layout">
    <section class="admin-surface admin-config-panel">
      <h2>公告与活动提示</h2>
      <p>选择展示位置并编辑内容。留空保存可隐藏对应提示。</p>

      <form @submit.prevent="saveNotice">
        <label for="notice-type">展示位置</label>
        <select id="notice-type" v-model="noticeType" :disabled="noticeSaving" @change="loadNotice">
          <option v-for="item in noticeTypes" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
        <label for="notice-content">提示内容</label>
        <div v-if="noticeLoading" class="admin-textarea-skeleton" aria-label="正在加载公告" />
        <textarea v-else id="notice-content" v-model="noticeContent" maxlength="10000" rows="8" placeholder="输入要展示给用户的内容" @input="noticeError = ''" />
        <div class="admin-field-meta"><span>{{ noticeContent.length }} / 10000</span><span>保存后用户端立即生效</span></div>
        <p v-if="noticeError" class="admin-form-error" role="alert">{{ noticeError }}</p>
        <button class="admin-primary-button" type="submit" :disabled="noticeLoading || noticeSaving"><Save :size="17" />{{ noticeSaving ? '正在保存…' : '保存提示内容' }}</button>
      </form>
    </section>

    <section class="admin-surface admin-config-panel">
      <h2>交流群二维码</h2>
      <p>从手机相册选择二维码图片，上传后用户点击底部“交流群”即可查看。</p>

      <div class="admin-qr-preview" :class="{ 'admin-qr-preview--empty': !displayedQRCode }">
        <img v-if="displayedQRCode" :src="displayedQRCode" :alt="selectedQRCode ? '待上传的交流群二维码预览' : '当前交流群二维码预览'" />
        <template v-else><Image :size="30" /><span>{{ qrcodeLoading ? '正在加载预览' : '暂未配置二维码' }}</span></template>
      </div>

      <form @submit.prevent="uploadQRCode">
        <label class="admin-upload-control" :class="{ 'admin-upload-control--disabled': qrcodeLoading || qrcodeSaving }" for="qrcode-file">
          <input id="qrcode-file" ref="qrcodeFileInput" type="file" accept="image/png,image/jpeg" :disabled="qrcodeLoading || qrcodeSaving" @change="selectQRCode" />
          <span class="admin-upload-icon"><FileUp :size="20" /></span>
          <span class="admin-upload-copy">
            <strong>{{ selectedQRCode ? selectedQRCode.name : '选择二维码图片' }}</strong>
            <small>支持 PNG、JPG，最大 {{ qrcodeMaxUploadMB }} MB</small>
          </span>
        </label>
        <p v-if="qrcodeError" class="admin-form-error" role="alert">{{ qrcodeError }}</p>
        <p v-else class="admin-form-helper">请选择清晰、完整且没有遮挡的二维码图片。</p>
        <div class="admin-qr-actions">
          <button class="admin-primary-button" type="submit" :disabled="qrcodeLoading || qrcodeSaving || !selectedQRCode"><FileUp :size="17" />{{ qrcodeSaving ? '正在上传…' : '上传并保存' }}</button>
          <button v-if="selectedQRCode" class="admin-secondary-button" type="button" :disabled="qrcodeSaving" @click="clearSelectedQRCode()"><X :size="17" />取消选择</button>
          <button v-else-if="qrcode" class="admin-secondary-button admin-remove-button" type="button" :disabled="qrcodeSaving" @click="removeQRCode"><Trash2 :size="17" />移除当前二维码</button>
        </div>
      </form>
    </section>
  </div>
</template>
