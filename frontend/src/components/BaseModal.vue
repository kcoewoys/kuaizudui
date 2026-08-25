<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { X } from 'lucide-vue-next'

const props = withDefaults(defineProps<{ open: boolean; size?: 'small' | 'normal' | 'wide' }>(), {
  size: 'normal',
})

const emit = defineEmits<{ close: [] }>()
const modalSheet = ref<HTMLElement | null>(null)
const titleId = `modal-title-${useId()}`
const subtitleId = `modal-subtitle-${useId()}`
let previouslyFocused: HTMLElement | null = null

function focusableElements() {
  if (!modalSheet.value) return []

  return Array.from(
    modalSheet.value.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => element.getClientRects().length > 0)
}

function closeModal() {
  emit('close')
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    closeModal()
    return
  }

  if (event.key !== 'Tab') return

  const elements = focusableElements()
  if (elements.length === 0) {
    event.preventDefault()
    modalSheet.value?.focus()
    return
  }

  const first = elements[0]
  const last = elements[elements.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(
  () => props.open,
  async (open) => {
    if (open) {
      previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null
      await nextTick()
      const elements = focusableElements()
      ;(elements[0] ?? modalSheet.value)?.focus()
      return
    }

    if (previouslyFocused) {
      await nextTick()
      if (previouslyFocused.isConnected) previouslyFocused.focus()
      previouslyFocused = null
    }
  },
)

onBeforeUnmount(() => {
  if (props.open && previouslyFocused?.isConnected) previouslyFocused.focus()
})
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="open" class="modal-backdrop" @click.self="closeModal">
        <section
          ref="modalSheet"
          class="modal-sheet"
          :class="`modal-sheet--${size}`"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          :aria-describedby="$slots.subtitle ? subtitleId : undefined"
          tabindex="-1"
          @keydown="handleKeydown"
        >
          <header class="modal-header">
            <div class="modal-heading">
              <h2 :id="titleId"><slot name="title" /></h2>
              <p v-if="$slots.subtitle" :id="subtitleId"><slot name="subtitle" /></p>
            </div>
            <button class="icon-button" type="button" aria-label="关闭" @click="closeModal">
              <X :size="22" :stroke-width="2" />
            </button>
          </header>
          <div class="modal-body">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="modal-footer">
            <slot name="footer" />
          </footer>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>
