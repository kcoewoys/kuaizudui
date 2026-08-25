<script setup lang="ts">
import { Check } from 'lucide-vue-next'

defineProps<{ message: string }>()
</script>

<template>
  <Transition name="toast">
    <div v-if="message" class="toast-message" role="status">
      <Check :size="17" :stroke-width="2.5" />
      <span>{{ message }}</span>
    </div>
  </Transition>
</template>
