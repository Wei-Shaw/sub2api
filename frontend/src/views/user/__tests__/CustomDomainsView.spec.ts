import { describe, expect, it, beforeEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { CustomDomain } from '@/api/customDomains'
import CustomDomainsView from '../CustomDomainsView.vue'

const {
  listUserCustomDomains,
  createCustomDomain,
  verifyCustomDomain,
  deleteCustomDomain,
  showError,
  showSuccess,
  copyToClipboard,
} = vi.hoisted(() => ({
  listUserCustomDomains: vi.fn(),
  createCustomDomain: vi.fn(),
  verifyCustomDomain: vi.fn(),
  deleteCustomDomain: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
}))

const messages: Record<string, string> = {
  'common.delete': 'Delete',
  'common.refresh': 'Refresh',
  'common.loading': 'Loading...',
  'common.cancel': 'Cancel',
  'customDomains.endpointIsolation': 'Endpoint isolation',
  'customDomains.title': 'Custom Domains',
  'customDomains.description': 'Use a verified hostname as your API base URL.',
  'customDomains.addDomain': 'Add Domain',
  'customDomains.addDomainDescription': 'Start with the API hostname.',
  'customDomains.domainLabel': 'Domain name',
  'customDomains.domainPlaceholder': 'api.example.com',
  'customDomains.domainHint': 'Use a dedicated API subdomain.',
  'customDomains.gatewayTarget': 'Gateway Target',
  'customDomains.gatewayTargetHint': 'Point your CNAME record to this gateway.',
  'customDomains.cnameTarget': 'CNAME Target',
  'customDomains.setupGuideTitle': 'Setup flow',
  'customDomains.setupGuideDescription': 'Each domain moves through checks.',
  'customDomains.setupSteps.addDomain.title': 'Enter the API hostname',
  'customDomains.setupSteps.addDomain.description': 'Add the hostname you want to use.',
  'customDomains.setupSteps.addTxt.title': 'Add the TXT ownership record',
  'customDomains.setupSteps.addTxt.description': 'Copy the generated TXT name and value.',
  'customDomains.setupSteps.addCname.title': 'Add the CNAME routing record',
  'customDomains.setupSteps.addCname.description': 'Point the hostname to the gateway target.',
  'customDomains.setupSteps.verify.title': 'Run verification',
  'customDomains.setupSteps.verify.description': 'Click Verify after DNS propagates.',
  'customDomains.yourDomains': 'Domain status',
  'customDomains.yourDomainsDescription': 'Track verification and DNS records.',
  'customDomains.activeCount': '{count} active',
  'customDomains.pendingCount': '{count} pending',
  'customDomains.createdAt': 'Added',
  'customDomains.dnsRecords': 'DNS Records',
  'customDomains.dnsRecordsDescription': 'Add both records at your DNS provider.',
  'customDomains.copyExactly': 'Copy exactly',
  'customDomains.recordName': 'Name',
  'customDomains.recordValue': 'Value',
  'customDomains.ownershipRecordTitle': 'Prove ownership',
  'customDomains.ownershipRecordHint': 'This TXT record proves ownership.',
  'customDomains.routingRecordTitle': 'Route API traffic',
  'customDomains.routingRecordHint': 'This CNAME sends requests to the gateway.',
  'customDomains.statusUpdateTitle': 'Status update',
  'customDomains.nextActionTitle': 'Next action',
  'customDomains.lastChecked': 'Last checked',
  'customDomains.neverChecked': 'Never checked',
  'customDomains.verifiedAt': 'Verified',
  'customDomains.verify': 'Verify',
  'customDomains.recheck': 'Recheck DNS',
  'customDomains.verifying': 'Verifying...',
  'customDomains.copyBaseUrl': 'Copy Base URL',
  'customDomains.apiBaseUrl': 'API Base URL',
  'customDomains.apiBaseUrlHint': 'Use this base URL with your API keys.',
  'customDomains.empty': 'No custom domains',
  'customDomains.emptyDescription': 'Add a domain to get records.',
  'customDomains.disabled': 'Custom domains are currently disabled',
  'customDomains.disabledHint': 'An administrator must enable this feature.',
  'customDomains.created': 'Domain added',
  'customDomains.createdNeedsDns': 'Domain added. Add the TXT and CNAME records below, then run verification.',
  'customDomains.deleted': 'Domain deleted',
  'customDomains.verified': 'Domain verified',
  'customDomains.verifiedInline': 'Verification passed. This hostname is ready to use as an API base URL.',
  'customDomains.verifyPending': 'DNS verification is still pending',
  'customDomains.verifyPendingInline': 'Verification ran, but DNS is not ready yet.',
  'customDomains.loadFailed': 'Failed to load custom domains',
  'customDomains.saveFailed': 'Failed to save custom domain',
  'customDomains.deleteConfirmTitle': 'Delete Custom Domain',
  'customDomains.deleteConfirmMessage': 'Delete {domain}?',
  'customDomains.statuses.pending_dns': 'Pending DNS',
  'customDomains.statuses.active': 'Active',
  'customDomains.statuses.disabled': 'Disabled',
  'customDomains.statuses.error': 'Error',
  'customDomains.statusMessages.pending_dns.title': 'Waiting for DNS records',
  'customDomains.statusMessages.pending_dns.description': 'Add the TXT and CNAME records, then run a verification check.',
  'customDomains.statusMessages.active.title': 'Domain verified',
  'customDomains.statusMessages.active.description': 'This hostname is active.',
  'customDomains.statusMessages.disabled.title': 'Domain disabled',
  'customDomains.statusMessages.disabled.description': 'This hostname is not accepting traffic.',
  'customDomains.statusMessages.error.title': 'Verification needs attention',
  'customDomains.statusMessages.error.description': 'The last check could not confirm DNS.',
  'customDomains.nextActions.pending_dns': 'Add or confirm both DNS records, wait for propagation, then click Verify.',
  'customDomains.nextActions.active': 'Copy the API Base URL and use it anywhere you call the shared endpoint.',
  'customDomains.nextActions.disabled': 'Contact an administrator to re-enable this hostname.',
  'customDomains.nextActions.error': 'Compare the DNS records with your provider settings, then recheck.',
}

vi.mock('@/api/customDomains', () => {
  const customDomainsAPI = {
    listUserCustomDomains,
    createCustomDomain,
    verifyCustomDomain,
    deleteCustomDomain,
  }

  return {
    customDomainsAPI,
    default: customDomainsAPI,
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        let value = messages[key] ?? key
        if (params) {
          Object.entries(params).forEach(([param, replacement]) => {
            value = value.replace(`{${param}}`, String(replacement))
          })
        }
        return value
      },
    }),
  }
})

const pendingDomain = (): CustomDomain => ({
  id: 1,
  user_id: 7,
  all_users: false,
  user_ids: [7],
  domain: 'api.customer.test',
  status: 'pending_dns',
  verification_txt_name: '_sub2api-verify.api.customer.test',
  verification_txt_value: 'sub2api-domain-verification=token-123',
  cname_target: 'gateway.sub2api.test',
  last_error: null,
  verified_at: null,
  last_checked_at: null,
  disabled_at: null,
  disabled_reason: null,
  created_at: '2026-07-08T00:00:00Z',
  updated_at: '2026-07-08T00:00:00Z',
  can_manage: true,
})

function mountView() {
  return mount(CustomDomainsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        ConfirmDialog: { template: '<div />' },
        EmptyState: {
          props: ['title', 'description', 'actionText'],
          emits: ['action'],
          template: '<div><h3>{{ title }}</h3><p>{{ description }}</p><button v-if="actionText" @click="$emit(\'action\')">{{ actionText }}</button></div>',
        },
        Icon: {
          props: ['name'],
          template: '<span :data-icon="name" />',
        },
      },
    },
  })
}

describe('CustomDomainsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listUserCustomDomains.mockResolvedValue({
      enabled: true,
      cname_target: 'gateway.sub2api.test',
      domains: [pendingDomain()],
    })
  })

  it('renders setup instructions and copy-ready DNS records', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Setup flow')
    expect(wrapper.text()).toContain('Prove ownership')
    expect(wrapper.text()).toContain('_sub2api-verify.api.customer.test')
    expect(wrapper.text()).toContain('sub2api-domain-verification=token-123')
    expect(wrapper.text()).toContain('Route API traffic')
    expect(wrapper.text()).toContain('gateway.sub2api.test')
    expect(wrapper.text()).toContain('Waiting for DNS records')
    expect(wrapper.text()).toContain('Status update')
    expect(wrapper.text()).toContain('Last checked')
  })

  it('updates the inline status panel after verification succeeds', async () => {
    verifyCustomDomain.mockResolvedValue({
      ...pendingDomain(),
      status: 'active',
      verified_at: '2026-07-08T00:05:00Z',
      last_checked_at: '2026-07-08T00:05:00Z',
      last_error: null,
    })

    const wrapper = mountView()
    await flushPromises()

    const verifyButton = wrapper.findAll('button').find((button) => button.text() === 'Verify')
    expect(verifyButton).toBeTruthy()

    await verifyButton!.trigger('click')
    await flushPromises()

    expect(verifyCustomDomain).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('Verification passed. This hostname is ready to use as an API base URL.')
    expect(wrapper.text()).toContain('Domain verified')
    expect(wrapper.text()).toContain('https://api.customer.test')
  })
})
