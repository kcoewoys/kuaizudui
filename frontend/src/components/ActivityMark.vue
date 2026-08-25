<script setup lang="ts">
import type { ActivityIcon } from '@/domain/activities'

const props = defineProps<{ icon: ActivityIcon | 'lucky'; compact?: boolean }>()
const imageSources: Record<ActivityIcon | 'lucky', string> = {
  lucky: '/activity-icons/lucky.png',
  grocery: '/activity-icons/grocery.png',
  turntable: '/activity-icons/turntable.png',
  monopoly: '/activity-icons/monopoly.png',
  daily: '/activity-icons/daily.png',
}
</script>

<template>
  <span class="activity-mark" :class="{ 'activity-mark--compact': compact }" aria-hidden="true">
    <img class="activity-image" :src="imageSources[props.icon]" alt="" draggable="false" />
  </span>
</template>
