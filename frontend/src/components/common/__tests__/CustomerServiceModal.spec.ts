import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import CustomerServiceModal from '@/components/common/CustomerServiceModal.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'common.customerService': 'Online Support',
      'common.customerServiceDescription': 'Contact us',
      'common.closeCustomerService': 'Close online support',
      'common.afterSalesSupport': 'Customer Service',
      'common.officialGroup': 'Official Group',
      'common.openSupportLink': 'Open support link',
      'common.openGroupLink': 'Join official group',
    }[key] || key),
  }),
}))

vi.mock('qrcode', () => ({
  default: {
    toDataURL: vi.fn(async () => 'data:image/png;base64,generated'),
  },
}))

function mountModal(props: Record<string, string>) {
  return mount(CustomerServiceModal, {
    attachTo: document.body,
    props,
  })
}

describe('CustomerServiceModal', () => {
  afterEach(() => {
    document.body.style.overflow = ''
    document.body.innerHTML = ''
  })

  it('opens the configured support cards and closes with Escape', async () => {
    const wrapper = mountModal({
      afterSalesQrCode: '/support.png',
      afterSalesLink: 'https://t.me/support',
      officialGroupQrCode: '/group.png',
      officialGroupLink: 'https://t.me/group',
    })

    await wrapper.get('button[aria-haspopup="dialog"]').trigger('click')
    await nextTick()

    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog?.textContent).toContain('Customer Service')
    expect(dialog?.textContent).toContain('Official Group')
    expect(document.body.style.overflow).toBe('hidden')

    const links = Array.from(dialog?.querySelectorAll('a') ?? [])
    expect(links).toHaveLength(2)
    expect(links[0].getAttribute('href')).toBe('https://t.me/support')
    expect(links[0].getAttribute('target')).toBe('_blank')
    expect(links[0].getAttribute('rel')).toBe('noopener noreferrer')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    await new Promise((resolve) => window.setTimeout(resolve, 200))
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    expect(document.body.style.overflow).toBe('')

    wrapper.unmount()
  })

  it('hides the entry when every configured value is unsafe', async () => {
    const wrapper = mountModal({
      afterSalesLink: 'javascript:alert(1)',
      afterSalesQrCode: '//evil.example/qr.png',
    })

    await nextTick()
    expect(wrapper.find('button[aria-haspopup="dialog"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
