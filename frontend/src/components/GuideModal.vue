<script setup lang="ts">
import { ChevronDown } from 'lucide-vue-next'
import BaseModal from './BaseModal.vue'

defineProps<{
  open: boolean
  title: string
  steps: string[]
  note?: string
}>()

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <BaseModal :open="open" size="wide" @close="emit('close')">
    <template #title>{{ title }}</template>
    <ol class="guide-list">
      <li v-for="(step, index) in steps" :key="step">
        <div class="guide-step">
          <span class="guide-index">{{ String(index + 1).padStart(2, '0') }}</span>
          <span class="guide-dot" aria-hidden="true" />
          <span>{{ step }}</span>
        </div>
        <ChevronDown v-if="index < steps.length - 1" class="guide-chevron" :size="15" />
      </li>
    </ol>
    <div v-if="note" class="guide-note">{{ note }}</div>
    <button class="primary-button full-width" type="button" @click="emit('close')">我知道了</button>
  </BaseModal>
</template>
