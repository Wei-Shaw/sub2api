import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({
  getEntry: vi.fn(),
  fetchOne: vi.fn(),
}))

vi.mock('@/utils/ipGeoLookup', () => ({
  getEntry: mocks.getEntry,
  fetchOne: mocks.fetchOne,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        const table: Record<string, string> = {
          'usage.ipGeo.fetch': 'Fetch region',
          'usage.ipGeo.fetching': 'Fetching...',
          'usage.ipGeo.failed': 'Failed',
          'usage.ipGeo.private': 'Private address',
          'usage.ipGeo.refreshTitle': 'Refresh',
          'usage.ipGeo.detailOrg': 'ISP',
          'usage.ipGeo.detailTimezone': 'Timezone',
          'usage.ipGeo.detailAccuracy': 'Accuracy',
          'usage.ipGeo.detailCoordinates': 'Coordinates',
        }
        return table[key] ?? key
      },
    }),
  }
})

import IpGeoCell from '../IpGeoCell.vue'

describe('IpGeoCell', () => {
  beforeEach(() => {
    mocks.getEntry.mockReset()
    mocks.fetchOne.mockReset()
  })

  it('renders a clickable fetch link in idle state and triggers fetchOne on click', async () => {
    mocks.getEntry.mockReturnValue({ status: 'idle' })
    const wrapper = mount(IpGeoCell, { props: { ip: '8.8.8.8' } })
    expect(wrapper.text()).toContain('Fetch region')
    await wrapper.find('button').trigger('click')
    expect(mocks.fetchOne).toHaveBeenCalledWith('8.8.8.8')
  })

  it('renders loading state', () => {
    mocks.getEntry.mockReturnValue({ status: 'loading' })
    const wrapper = mount(IpGeoCell, { props: { ip: '8.8.8.8' } })
    expect(wrapper.text()).toContain('Fetching...')
  })

  it('renders success state with label, tooltip detail, and a refresh button', async () => {
    mocks.getEntry.mockReturnValue({
      status: 'success',
      label: 'CN · Guangdong · Shenzhen',
      detail: {
        organization: 'AS4134 Chinanet',
        timezone: 'Asia/Shanghai',
        accuracy: 10,
        latitude: '22.5',
        longitude: '114.0',
      },
    })
    const wrapper = mount(IpGeoCell, { props: { ip: '121.35.47.43' } })
    expect(wrapper.text()).toContain('CN · Guangdong · Shenzhen')
    const buttons = wrapper.findAll('button')
    expect(buttons.length).toBe(2)
    expect(buttons[0].attributes('title')).toContain('AS4134 Chinanet')
    expect(buttons[0].attributes('title')).toContain('Asia/Shanghai')
    await buttons[1].trigger('click')
    expect(mocks.fetchOne).toHaveBeenCalledWith('121.35.47.43', true)
  })

  it('opens the external lookup page when the label is clicked', async () => {
    mocks.getEntry.mockReturnValue({ status: 'success', label: 'US · California', detail: {} })
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    const wrapper = mount(IpGeoCell, { props: { ip: '8.8.4.4' } })
    await wrapper.findAll('button')[0].trigger('click')
    expect(openSpy).toHaveBeenCalledWith(
      'https://www.iplocation.net/ip-lookup?query=8.8.4.4',
      '_blank',
      'noopener,noreferrer'
    )
    openSpy.mockRestore()
  })

  it('renders failed state as a clickable retry', async () => {
    mocks.getEntry.mockReturnValue({ status: 'error' })
    const wrapper = mount(IpGeoCell, { props: { ip: '8.8.8.8' } })
    expect(wrapper.text()).toContain('Failed')
    await wrapper.find('button').trigger('click')
    expect(mocks.fetchOne).toHaveBeenCalledWith('8.8.8.8')
  })

  it('renders private state as non-clickable text', () => {
    mocks.getEntry.mockReturnValue({ status: 'private' })
    const wrapper = mount(IpGeoCell, { props: { ip: '192.168.1.1' } })
    expect(wrapper.text()).toContain('Private address')
    expect(wrapper.find('button').exists()).toBe(false)
  })
})
