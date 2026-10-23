import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import CompactCapabilitySettings from '../CompactCapabilitySettings.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        key === 'admin.accounts.openai.compactTargetRecords'
          ? `${key}:${params?.count}`
          : key
    })
  }
})

const SelectStub = {
  props: ['modelValue', 'options', 'disabled'],
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      :disabled="disabled"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
}

function mountPanel(extraProps: Record<string, unknown> = {}) {
  return mount(CompactCapabilitySettings, {
    props: {
      idPrefix: 'test',
      mode: 'auto',
      summaryEnabled: false,
      ...extraProps
    } as any,
    global: {
      stubs: { Select: SelectStub }
    }
  })
}

function probeRecord(index: number) {
  return {
    version: 1,
    target: {
      kind: 'native_v2',
      requested_model: `requested-${index}`,
      model: `model-${index}`,
      protocol: 'responses',
      endpoint: `https://user:fixture-secret@api.deepseek.com/responses?api_key=fixture-secret#fragment`,
      endpoint_hash: `endpoint-hash-${index}`,
      logical_account_id: 1,
      credential_account_id: 2,
      credential_source: 'credential_account:2',
      auth_mode: 'apikey',
      config_fingerprint: `fixture-fingerprint-${index}`,
      fingerprint_version: 1
    },
    verdict: index === 9 ? 'supported' : 'unknown',
    last_checked_at: `2026-10-${String(index + 1).padStart(2, '0')}T10:00:00Z`,
    verified_at: index === 9 ? '2026-10-10T10:00:00Z' : undefined,
    error_class: index === 9 ? '' : 'rate_limited'
  }
}

describe('CompactCapabilitySettings', () => {
  it('renders a bounded newest-first target history without credential material or secrets', () => {
    const targets = Object.fromEntries(
      Array.from({ length: 10 }, (_, index) => [`target-${index}`, probeRecord(index)])
    )
    const wrapper = mountPanel({
      mode: 'force_on',
      summaryEnabled: true,
      extra: {
        openai_compact_probe_targets: targets,
        openai_compact_supported: false,
        openai_compact_checked_at: '2026-10-01T10:00:00Z'
      },
      showLegacyEvidence: true,
      showProbeHistory: true
    })

    expect(wrapper.get('[data-testid="compact-force-on-hint"]').text()).toContain(
      'admin.accounts.openai.compactModeForceOnHint'
    )
    expect(wrapper.get('[data-testid="compact-legacy-evidence"]').text()).toContain(
      'admin.accounts.openai.compactUnsupported'
    )
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(8)
    expect(rows[0].text()).toContain('model-9')
    expect(rows[0].text()).toContain('https://api.deepseek.com/responses')
    expect(wrapper.html()).not.toContain('fixture-secret')
    expect(wrapper.html()).not.toContain('fixture-fingerprint')
    expect(wrapper.html()).not.toContain('endpoint-hash')
    expect(wrapper.get<HTMLInputElement>('#test-compact-summary-enabled').element.checked).toBe(true)
  })

  it('reports an empty provider declaration as unknown rather than unsupported', () => {
    const wrapper = mountPanel({
      platform: 'kimi',
      accountMode: 'payg',
      showProviderDeclaration: true
    })

    expect(wrapper.get('[data-testid="compact-provider-declaration"]').text()).toContain(
      'admin.accounts.openai.compactProviderUnknown'
    )
  })

  it('keeps model and native/legacy target results isolated in history', () => {
    const native = probeRecord(0)
    native.target.model = 'model-native'
    native.verdict = 'supported'
    const legacy = probeRecord(1)
    legacy.target.kind = 'legacy_compact'
    legacy.target.model = 'model-legacy'
    legacy.verdict = 'unsupported'
    const wrapper = mountPanel({
      showProbeHistory: true,
      extra: {
        openai_compact_probe_targets: {
          native,
          legacy
        }
      }
    })

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('admin.accounts.openai.compactKindLegacy')
    expect(rows[0].text()).toContain('model-legacy')
    expect(rows[0].text()).toContain('admin.accounts.openai.compactVerdictUnsupported')
    expect(rows[1].text()).toContain('admin.accounts.openai.compactKindNativeV2')
    expect(rows[1].text()).toContain('model-native')
    expect(rows[1].text()).toContain('admin.accounts.openai.compactVerdictSupported')
  })

  it('keeps summary consent enabled independently when native attempts are forced off', () => {
    const wrapper = mountPanel({
      mode: 'force_off',
      summaryEnabled: true,
      showLegacyEvidence: true,
      extra: { openai_compact_supported: true }
    })

    expect(wrapper.get('[data-testid="compact-force-off-hint"]').text()).toContain(
      'admin.accounts.openai.compactModeForceOffHint'
    )
    expect(wrapper.get<HTMLInputElement>('#test-compact-summary-enabled').element.checked).toBe(true)
  })
})
