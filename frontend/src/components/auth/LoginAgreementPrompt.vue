<template>
  <!--
    Checkbox mode. Inline consent that sits inside the login/register form, so
    it has to read as one line of body copy with the document titles as links —
    not as a panel. The links are the only accent here, which is exactly what
    accent is for.
  -->
  <div v-if="mode === 'checkbox' && documents.length > 0" class="flex items-start gap-2">
    <!--
      A native checkbox, tinted with the accent token. `appearance-none` plus a
      hand-drawn tick would cost us the UA's own focus ring and indeterminate
      handling for nothing: at 14px the platform control is the right mark.
    -->
    <input
      id="login-agreement-consent"
      type="checkbox"
      :checked="props.accepted"
      class="mt-0.5 h-3.5 w-3.5 shrink-0 [accent-color:rgb(var(--ds-accent-solid))]"
      @change="handleCheckboxChange"
    />
    <p class="text-xs text-ink-secondary">
      <label for="login-agreement-consent" class="cursor-pointer">
        {{ t('legal.loginAgreementPrompt.checkboxPrefix') }}
      </label>
      <template v-for="(doc, index) in documents" :key="doc.id || doc.title">
        <RouterLink
          :to="documentRoute(doc)"
          target="_blank"
          rel="noopener noreferrer"
          class="text-accent underline-offset-2 transition-colors duration-fast hover:text-accent-hover hover:underline"
        >
          {{ doc.title }}
        </RouterLink>
        <span v-if="index < documents.length - 1">
          {{ t('legal.loginAgreementPrompt.documentSeparator') }}
        </span>
      </template>
    </p>
  </div>

  <!--
    Modal-mode notice, shown once the dialog has been dismissed without an
    acceptance. This was an accent-tinted card with an accent shield glyph —
    accent standing in for "blocked", which is the one thing accent must never
    say. It is now a hairline row: the copy carries the state and the only
    interactive element is the button.
  -->
  <div
    v-else-if="!props.accepted && documents.length > 0"
    class="flex items-start justify-between gap-3 border border-line p-3"
    data-testid="login-agreement-notice"
  >
    <div class="min-w-0">
      <p class="text-sm font-medium text-ink">
        {{ t('legal.loginAgreementPrompt.noticeTitle') }}
      </p>
      <p class="mt-1 text-xs text-ink-tertiary">
        {{ t('legal.loginAgreementPrompt.noticeDescription') }}
      </p>
    </div>
    <Button
      variant="outline"
      size="sm"
      class="shrink-0"
      data-testid="login-agreement-open"
      @click="emit('open')"
    >
      {{ t('legal.loginAgreementPrompt.viewTerms') }}
    </Button>
  </div>

  <!--
    The dialog is now `BaseDialog` rather than a hand-rolled Teleport. What that
    buys, none of which the local copy had: `role="dialog"`, `aria-modal`,
    `aria-labelledby` on the title, focus moved into the panel and restored on
    close, body scroll lock, and the shared 220ms/150ms modal transition. What
    it drops: the `backdrop-blur-sm` scrim, the 2xl radii, the `shadow-2xl`, and
    the `hover:-translate-y-0.5` document tiles.

    Escape resolves to REJECT, not to a silent dismissal — a consent gate that
    can be closed without an answer is a gate that has been answered "yes" by
    accident. Clicking the scrim does nothing, so a stray click cannot reject.
  -->
  <BaseDialog
    :show="dialogVisible"
    :title="t('legal.loginAgreementPrompt.dialogTitle')"
    width="normal"
    :close-on-escape="true"
    :close-on-click-outside="false"
    :show-close-button="false"
    :z-index="140"
    @close="emit('reject')"
  >
    <div class="space-y-5">
      <p class="text-sm text-ink-secondary">
        {{
          t('legal.loginAgreementPrompt.dialogDescription', {
            date: props.updatedAt || t('legal.loginAgreementPrompt.recently')
          })
        }}
      </p>

      <!--
        The revision date used to appear twice: once inside the sentence above
        and once again as a pill next to the title. One channel is enough.
      -->
      <div>
        <p class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
          {{ t('legal.loginAgreementPrompt.relatedDocuments') }}
        </p>
        <ul class="mt-2 divide-y divide-line border-y border-line">
          <li v-for="doc in documents" :key="doc.id || doc.title">
            <RouterLink
              :to="documentRoute(doc)"
              target="_blank"
              rel="noopener noreferrer"
              class="flex items-center justify-between gap-3 px-1 py-2.5 text-sm text-ink transition-colors duration-fast hover:bg-surface-hover hover:text-accent"
            >
              <span class="truncate">{{ doc.title }}</span>
              <Icon name="externalLink" size="sm" class="shrink-0 text-ink-tertiary" />
            </RouterLink>
          </li>
        </ul>
      </div>
    </div>

    <template #footer>
      <Button variant="outline" size="md" data-testid="login-agreement-reject" @click="emit('reject')">
        {{ t('legal.loginAgreementPrompt.reject') }}
      </Button>
      <Button
        tone="accent"
        variant="solid"
        size="md"
        data-testid="login-agreement-accept"
        @click="emit('accept')"
      >
        {{ t('legal.loginAgreementPrompt.accept') }}
      </Button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
// By path, not through `components/common/index.ts`: the barrel re-exports
// LocaleSwitcher, which pulls `createI18n` into the graph and breaks the specs
// that mock `vue-i18n` with a partial factory.
import BaseDialog from '@/components/common/BaseDialog.vue'
import Button from '@/components/common/Button.vue'
import Icon from '@/components/icons/Icon.vue'
import type { LoginAgreementDocument } from '@/types'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  accepted: boolean
  documents: LoginAgreementDocument[]
  mode: 'modal' | 'checkbox' | string
  updatedAt?: string
  visible: boolean
}>(), {
  updatedAt: ''
})

const emit = defineEmits<{
  accept: []
  reject: []
  open: []
}>()

// Declared before `dialogVisible`, which reads it. The previous order had the
// computed referencing `documents` a line before the `const` that defines it —
// legal only because a computed getter is lazy, and a trap for the next edit.
const documents = computed(() => props.documents.filter((doc) => doc.title.trim()))
const mode = computed(() => (props.mode === 'checkbox' ? 'checkbox' : 'modal'))
const dialogVisible = computed(() => props.visible && documents.value.length > 0)

function documentRoute(doc: LoginAgreementDocument) {
  return {
    name: 'LegalDocument',
    params: {
      documentId: doc.id || doc.title,
    },
  }
}

function handleCheckboxChange(event: Event): void {
  const checked = (event.target as HTMLInputElement).checked
  if (checked) {
    emit('accept')
  } else {
    emit('reject')
  }
}
</script>

<!--
  The `<style scoped>` block held the `.agreement-fade-*` classes for the
  hand-rolled Teleport transition. `BaseDialog` uses the shared `.modal-*`
  transition from style.css, which is already on the motion tokens, so the local
  copy — including its `scale(0.98)` pop — is gone.
-->
