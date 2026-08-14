<template>
  <!--
    First-run chrome. This view is never wrapped in AppLayout — there is no nav
    yet — so it owns its own page frame, and that frame is deliberately the same
    voice as AuthLayout: flat canvas, one hairline-ruled panel, left-aligned
    lockup, no logo tile.

    What this replaces: a `bg-gradient-to-br` ground, a `rounded-2xl`
    gradient-filled cog tile with a `shadow-lg`, a `rounded-2xl shadow-xl`
    floating card, four pastel `rounded-full` step bubbles joined by connector
    bars, and three `rounded-xl` gray wells on the review step.
  -->
  <div class="min-h-screen bg-canvas px-4 py-10 sm:py-16">
    <div class="mx-auto w-full max-w-3xl">
      <header>
        <!-- Position in the sequence, as a number rather than a progress bar. -->
        <p class="font-mono text-2xs uppercase tracking-[0.08em] tabular-nums text-ink-tertiary">
          {{ pad(currentStep + 1) }} / {{ pad(steps.length) }}
        </p>
        <h1 class="mt-3 text-xl font-semibold text-ink">{{ t('setup.title') }}</h1>
        <p class="mt-1.5 text-sm text-ink-tertiary">{{ t('setup.description') }}</p>
      </header>

      <!--
        Step indicator. Same idiom as `.tabs` in style.css: a rule under the
        strip and a 2px segment marking position. Deliberately NOT four circles
        on a connector — that pattern spends a large round shape and a fill
        colour on information that is one word plus one number.

        State is never carried by colour alone: a finished step swaps its number
        for a check glyph and keeps its written title, the current step carries
        `aria-current` and the only accent on the strip.
      -->
      <ol class="mt-8 flex items-stretch border-b border-line" data-testid="setup-steps">
        <li
          v-for="(step, index) in steps"
          :key="step.id"
          :aria-current="currentStep === index ? 'step' : undefined"
          class="-mb-px min-w-0 flex-1 border-b-2 pb-2.5 pr-4 transition-colors duration-fast ease-out last:pr-0"
          :class="
            currentStep === index
              ? 'border-accent'
              : currentStep > index
                ? 'border-line-emphasis'
                : 'border-transparent'
          "
        >
          <span class="flex min-w-0 items-center gap-1.5">
            <Icon
              v-if="currentStep > index"
              name="check"
              size="xs"
              class="shrink-0 text-ink-secondary"
              :stroke-width="2"
            />
            <span
              v-else
              class="shrink-0 font-mono text-2xs tabular-nums"
              :class="currentStep === index ? 'text-accent' : 'text-ink-disabled'"
            >
              {{ pad(index + 1) }}
            </span>
            <span
              class="hidden truncate text-xs sm:block"
              :class="
                currentStep === index
                  ? 'font-medium text-ink'
                  : currentStep > index
                    ? 'text-ink-secondary'
                    : 'text-ink-disabled'
              "
            >
              {{ step.title }}
            </span>
          </span>
        </li>
      </ol>

      <div class="mt-6 border border-line bg-surface p-6">
        <!--
          Step change is opacity plus 4px of vertical travel. It was worth having
          at all only because the panel is the one thing on the page that
          changes; a horizontal slide of the whole surface would be the sort of
          full-screen gesture this system does not do.
        -->
        <Transition
          mode="out-in"
          enter-active-class="transition-[opacity,transform] duration-base ease-out"
          enter-from-class="translate-y-1 opacity-0"
          enter-to-class="translate-y-0 opacity-100"
          leave-active-class="transition-opacity duration-fast ease-in"
          leave-from-class="opacity-100"
          leave-to-class="opacity-0"
        >
          <!-- Step 1: Database -->
          <section v-if="currentStep === 0" key="database" data-testid="setup-step-database">
            <h2 class="text-md font-semibold text-ink">{{ t('setup.database.title') }}</h2>
            <p class="mt-1 text-sm text-ink-tertiary">{{ t('setup.database.description') }}</p>

            <div class="mt-5 grid gap-x-4 gap-y-2 sm:grid-cols-2">
              <FormField :label="t('setup.database.host')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.database.host"
                    type="text"
                    class="input"
                    placeholder="localhost"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.database.port')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model.number="formData.database.port"
                    type="number"
                    class="input font-mono tabular-nums"
                    placeholder="5432"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.database.username')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.database.user"
                    type="text"
                    class="input"
                    placeholder="postgres"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.database.password')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.database.password"
                    type="password"
                    autocomplete="off"
                    class="input"
                    :placeholder="t('setup.database.passwordPlaceholder')"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.database.databaseName')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.database.dbname"
                    type="text"
                    class="input"
                    placeholder="sub2api"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.database.sslMode')">
                <template #default="{ id, describedBy }">
                  <Select
                    :id="id"
                    v-model="formData.database.sslmode"
                    :aria-describedby="describedBy"
                    :options="sslModeOptions"
                  />
                </template>
              </FormField>
            </div>

            <div class="mt-4 flex flex-wrap items-center gap-3 border-t border-line-subtle pt-4">
              <Button
                variant="outline"
                size="md"
                :loading="testingDb"
                data-testid="setup-test-database"
                @click="testDatabaseConnection"
              >
                {{ t('setup.status.testConnection') }}
              </Button>
              <!--
                Success is a status, so it is `success`, never the accent — and
                it is a dot WITH a word, not a green tick on its own. It also no
                longer overwrites the button's label, which used to make the
                control change width mid-click.
              -->
              <StatusDot
                v-if="dbConnected"
                tone="success"
                :label="t('setup.status.success')"
                data-testid="setup-database-ok"
              />
            </div>
          </section>

          <!-- Step 2: Redis -->
          <section v-else-if="currentStep === 1" key="redis" data-testid="setup-step-redis">
            <h2 class="text-md font-semibold text-ink">{{ t('setup.redis.title') }}</h2>
            <p class="mt-1 text-sm text-ink-tertiary">{{ t('setup.redis.description') }}</p>

            <div class="mt-5 grid gap-x-4 gap-y-2 sm:grid-cols-2">
              <FormField :label="t('setup.redis.host')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.redis.host"
                    type="text"
                    class="input"
                    placeholder="localhost"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.redis.port')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model.number="formData.redis.port"
                    type="number"
                    class="input font-mono tabular-nums"
                    placeholder="6379"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.redis.username')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.redis.username"
                    type="text"
                    class="input"
                    :placeholder="t('setup.redis.usernamePlaceholder')"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.redis.password')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.redis.password"
                    type="password"
                    autocomplete="off"
                    class="input"
                    :placeholder="t('setup.redis.passwordPlaceholder')"
                  />
                </template>
              </FormField>

              <FormField :label="t('setup.redis.database')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model.number="formData.redis.db"
                    type="number"
                    class="input font-mono tabular-nums"
                    placeholder="0"
                  />
                </template>
              </FormField>
            </div>

            <!--
              A switch row, ruled rather than boxed. `Toggle` renders a bare
              `role="switch"` button, so the visible text is wired to it through
              `aria-labelledby`/`aria-describedby` — before this it was a
              control announced with no name at all.
            -->
            <div
              class="mt-2 flex items-center justify-between gap-4 rounded border border-line px-3 py-2.5"
            >
              <div class="min-w-0">
                <p id="redis-tls-label" class="text-xs font-medium text-ink">
                  {{ t('setup.redis.enableTls') }}
                </p>
                <p id="redis-tls-hint" class="mt-0.5 text-2xs text-ink-tertiary">
                  {{ t('setup.redis.enableTlsHint') }}
                </p>
              </div>
              <Toggle
                v-model="formData.redis.enable_tls"
                aria-labelledby="redis-tls-label"
                aria-describedby="redis-tls-hint"
                data-testid="setup-redis-tls"
              />
            </div>

            <div class="mt-4 flex flex-wrap items-center gap-3 border-t border-line-subtle pt-4">
              <Button
                variant="outline"
                size="md"
                :loading="testingRedis"
                data-testid="setup-test-redis"
                @click="testRedisConnection"
              >
                {{ t('setup.status.testConnection') }}
              </Button>
              <StatusDot
                v-if="redisConnected"
                tone="success"
                :label="t('setup.status.success')"
                data-testid="setup-redis-ok"
              />
            </div>
          </section>

          <!-- Step 3: Admin -->
          <section v-else-if="currentStep === 2" key="admin" data-testid="setup-step-admin">
            <h2 class="text-md font-semibold text-ink">{{ t('setup.admin.title') }}</h2>
            <p class="mt-1 text-sm text-ink-tertiary">{{ t('setup.admin.description') }}</p>

            <div class="mt-5 max-w-sm">
              <FormField :label="t('setup.admin.email')">
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="formData.admin.email"
                    type="email"
                    autocomplete="username"
                    class="input"
                    placeholder="admin@example.com"
                  />
                </template>
              </FormField>

              <!--
                The 8-character rule used to have no visible channel at all: too
                short a password simply left "Next" dead, with nothing next to
                the field saying why. The rule is now a hint, and its violation
                is an error on the field that caused it.
              -->
              <FormField
                :label="t('setup.admin.password')"
                :hint="t('setup.admin.passwordPlaceholder')"
                :error="passwordLengthError"
              >
                <template #default="{ id, describedBy, invalid }">
                  <input
                    :id="id"
                    v-model="formData.admin.password"
                    type="password"
                    autocomplete="new-password"
                    :aria-describedby="describedBy"
                    :aria-invalid="invalid || undefined"
                    class="input"
                    :class="{ 'input-error': passwordLengthError }"
                  />
                </template>
              </FormField>

              <FormField
                :label="t('setup.admin.confirmPassword')"
                :error="passwordMismatchError"
              >
                <template #default="{ id, describedBy, invalid }">
                  <input
                    :id="id"
                    v-model="confirmPassword"
                    type="password"
                    autocomplete="new-password"
                    :aria-describedby="describedBy"
                    :aria-invalid="invalid || undefined"
                    class="input"
                    :class="{ 'input-error': passwordMismatchError }"
                    :placeholder="t('setup.admin.confirmPasswordPlaceholder')"
                  />
                </template>
              </FormField>
            </div>
          </section>

          <!-- Step 4: Review -->
          <section v-else key="ready" data-testid="setup-step-ready">
            <h2 class="text-md font-semibold text-ink">{{ t('setup.ready.title') }}</h2>
            <p class="mt-1 text-sm text-ink-tertiary">{{ t('setup.ready.description') }}</p>

            <!-- A ruled summary list. Was three stacked gray wells. -->
            <dl class="mt-5 divide-y divide-line-subtle border-y border-line-subtle">
              <div
                v-for="row in reviewRows"
                :key="row.label"
                class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 py-2.5"
              >
                <dt class="text-2xs uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ row.label }}
                </dt>
                <dd class="font-mono text-xs tabular-nums text-ink [overflow-wrap:anywhere]">
                  {{ row.value }}
                </dd>
              </div>
            </dl>
          </section>
        </Transition>

        <!-- Error -->
        <div
          v-if="errorMessage"
          role="alert"
          class="mt-6 flex items-start gap-2.5 rounded border border-danger/40 bg-danger-tint p-3"
          data-testid="setup-error"
        >
          <Icon name="exclamationCircle" size="sm" class="mt-px shrink-0 text-danger" />
          <p class="text-sm text-danger">{{ errorMessage }}</p>
        </div>

        <!-- Install progress / success -->
        <div
          v-if="installSuccess"
          aria-live="polite"
          class="mt-6 flex items-start gap-2.5 rounded border border-success/40 bg-success-tint p-3"
          data-testid="setup-success"
        >
          <span
            v-if="!serviceReady"
            class="spinner mt-px h-4 w-4 shrink-0 text-success"
            aria-hidden="true"
          />
          <Icon v-else name="checkCircle" size="sm" class="mt-px shrink-0 text-success" />
          <div class="min-w-0">
            <p class="text-sm font-medium text-success">{{ t('setup.status.completed') }}</p>
            <p class="mt-0.5 text-xs text-ink-secondary">
              {{ serviceReady ? t('setup.status.redirecting') : t('setup.status.restarting') }}
            </p>
          </div>
        </div>

        <!-- Navigation. Ruled footer bar, flush to the panel edges. -->
        <div
          class="-mx-6 -mb-6 mt-8 flex items-center justify-between gap-3 border-t border-line bg-surface-sunken px-6 py-4"
        >
          <Button
            v-if="currentStep > 0 && !installSuccess"
            variant="outline"
            size="md"
            data-testid="setup-back"
            @click="currentStep--"
          >
            <template #icon>
              <Icon name="chevronLeft" size="xs" :stroke-width="2" />
            </template>
            {{ t('common.back') }}
          </Button>
          <div v-else></div>

          <Button
            v-if="currentStep < 3"
            tone="accent"
            variant="solid"
            size="md"
            :disabled="!canProceed"
            data-testid="setup-next"
            @click="nextStep"
          >
            {{ t('common.next') }}
            <template #trailing>
              <Icon name="chevronRight" size="xs" :stroke-width="2" />
            </template>
          </Button>

          <!--
            The label no longer flips to "Installing…" mid-click; `Button`
            overlays a spinner on the reserved label box and sets `aria-busy`.
          -->
          <Button
            v-else-if="!installSuccess"
            tone="accent"
            variant="solid"
            size="md"
            :loading="installing"
            data-testid="setup-install"
            @click="performInstall"
          >
            {{ t('setup.status.completeInstallation') }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { testDatabase, testRedis, install, type InstallRequest } from '@/api/setup'
import { buildGatewayUrl } from '@/api/client'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import Select from '@/components/common/Select.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const steps = computed(() => [
  { id: 'database', title: t('setup.database.title') },
  { id: 'redis', title: t('setup.redis.title') },
  { id: 'admin', title: t('setup.admin.title') },
  { id: 'complete', title: t('setup.ready.title') }
])

const pad = (value: number): string => String(value).padStart(2, '0')

const currentStep = ref(0)
const errorMessage = ref('')
const installSuccess = ref(false)

// Connection test states
const testingDb = ref(false)
const testingRedis = ref(false)
const dbConnected = ref(false)
const redisConnected = ref(false)
const installing = ref(false)
const confirmPassword = ref('')
const serviceReady = ref(false)

const sslModeOptions = computed(() => [
  { value: 'disable', label: t('setup.database.ssl.disable') },
  { value: 'require', label: t('setup.database.ssl.require') },
  { value: 'verify-ca', label: t('setup.database.ssl.verifyCa') },
  { value: 'verify-full', label: t('setup.database.ssl.verifyFull') }
])

// Default server port
const getCurrentPort = (): number => {
  const port = window.location.port
  if (port) {
    return parseInt(port, 10)
  }

  return window.location.protocol === 'https:' ? 443 : 80
}

const formData = reactive<InstallRequest>({
  database: {
    host: 'localhost',
    port: 5432,
    user: 'postgres',
    password: '',
    dbname: 'sub2api',
    sslmode: 'disable'
  },
  redis: {
    host: 'localhost',
    port: 6379,
    username: '',
    password: '',
    db: 0,
    enable_tls: false
  },
  admin: {
    email: '',
    password: ''
  },
  server: {
    host: '0.0.0.0',
    port: getCurrentPort(), // Use current port from browser
    mode: 'release'
  }
})

const reviewRows = computed(() => [
  {
    label: t('setup.ready.database'),
    value: `${formData.database.user}@${formData.database.host}:${formData.database.port}/${formData.database.dbname}`
  },
  {
    label: t('setup.ready.redis'),
    value: `${formData.redis.host}:${formData.redis.port}`
  },
  {
    label: t('setup.ready.adminEmail'),
    value: formData.admin.email
  }
])

/**
 * Both messages are gated on the field being non-empty: an untouched field is
 * not yet wrong. The gating conditions are the same ones `canProceed` uses, so
 * a disabled "Next" always has a visible reason next to a field.
 */
const passwordLengthError = computed(() =>
  formData.admin.password && formData.admin.password.length < 8
    ? t('profile.passwordTooShort')
    : ''
)

const passwordMismatchError = computed(() =>
  confirmPassword.value && formData.admin.password !== confirmPassword.value
    ? t('setup.admin.passwordMismatch')
    : ''
)

const canProceed = computed(() => {
  switch (currentStep.value) {
    case 0:
      return dbConnected.value
    case 1:
      return redisConnected.value
    case 2:
      return (
        formData.admin.email &&
        formData.admin.password.length >= 8 &&
        formData.admin.password === confirmPassword.value
      )
    default:
      return true
  }
})

async function testDatabaseConnection() {
  testingDb.value = true
  errorMessage.value = ''
  dbConnected.value = false

  try {
    await testDatabase(formData.database)
    dbConnected.value = true
  } catch (error: unknown) {
    const err = error as { response?: { data?: { detail?: string; message?: string } }; message?: string }
    errorMessage.value =
      err.response?.data?.detail || err.response?.data?.message || err.message || 'Connection failed'
  } finally {
    testingDb.value = false
  }
}

async function testRedisConnection() {
  testingRedis.value = true
  errorMessage.value = ''
  redisConnected.value = false

  try {
    await testRedis(formData.redis)
    redisConnected.value = true
  } catch (error: unknown) {
    const err = error as { response?: { data?: { detail?: string; message?: string } }; message?: string }
    errorMessage.value =
      err.response?.data?.detail || err.response?.data?.message || err.message || 'Connection failed'
  } finally {
    testingRedis.value = false
  }
}

function nextStep() {
  if (canProceed.value) {
    errorMessage.value = ''
    currentStep.value++
  }
}

async function performInstall() {
  installing.value = true
  errorMessage.value = ''

  try {
    await install(formData)
    installSuccess.value = true
    // Start polling for service restart
    waitForServiceRestart()
  } catch (error: unknown) {
    const err = error as { response?: { data?: { detail?: string; message?: string } }; message?: string }
    errorMessage.value =
      err.response?.data?.detail || err.response?.data?.message || err.message || 'Installation failed'
  } finally {
    installing.value = false
  }
}

// Wait for service to restart and become available
async function waitForServiceRestart() {
  const maxAttempts = 60 // Increase to 60 attempts, ~60 seconds max
  const interval = 1000 // 1 second between attempts

  // Wait a moment for the service to start restarting
  await new Promise((resolve) => setTimeout(resolve, 3000))

  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    try {
      // Use setup status endpoint as it tells us the real mode
      // Service might return 404 or connection refused while restarting
      const response = await fetch(buildGatewayUrl('/setup/status'), {
        method: 'GET',
        cache: 'no-store'
      })

      if (response.ok) {
        const data = await response.json()
        // If needs_setup is false, service has restarted in normal mode
        if (data.data && !data.data.needs_setup) {
          serviceReady.value = true
          // Redirect to login page after a short delay
          setTimeout(() => {
            window.location.href = '/login'
          }, 1500)
          return
        }
      }
    } catch {
      // Service not ready or network error during restart, continue polling
    }

    await new Promise((resolve) => setTimeout(resolve, interval))
  }

  // If we reach here, service didn't restart in time
  // Show a message to refresh manually
  errorMessage.value = t('setup.status.timeout')
}
</script>
