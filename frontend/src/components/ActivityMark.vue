<script setup lang="ts">
import type { ActivityIcon } from '@/domain/activities'

const props = defineProps<{ icon: ActivityIcon | 'lucky'; compact?: boolean }>()
const imageSources: Record<ActivityIcon | 'lucky', string> = {
  lucky: '/activity-icons/lucky.webp',
  grocery: '/activity-icons/grocery.webp',
  turntable: '/activity-icons/turntable.webp',
  monopoly: '/activity-icons/monopoly.webp',
  daily: '/activity-icons/daily.webp',
}
</script>

<template>
  <span class="activity-mark" :class="{ 'activity-mark--compact': compact }" aria-hidden="true">
    <img class="activity-image" :src="imageSources[props.icon]" alt="" draggable="false" />
  </span>
</template>
