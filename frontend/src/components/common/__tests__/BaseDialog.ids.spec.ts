import { afterEach, describe, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'

enableAutoUnmount(afterEach)

function openDialog(title: string) {
  return mount(BaseDialog, { props: { show: true, title }, global: { stubs: { Icon: true } } })
}

describe('dialog accessible titles', () => {
  it('associates simultaneous dialogs with their own title', async () => {
    openDialog('Edit account')
    openDialog('Confirm deletion')
    await nextTick()
    const dialogs = Array.from(document.body.querySelectorAll('[role="dialog"]'))
    expect(dialogs).toHaveLength(2)
    const titles = dialogs.map(dialog => document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent)
    expect(titles).toEqual(['Edit account', 'Confirm deletion'])
  })

  it('keeps the title association stable when a dialog is reopened', async () => {
    const wrapper = openDialog('Details')
    await nextTick()
    const id = document.body.querySelector('[role="dialog"]')!.getAttribute('aria-labelledby')!
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, title: 'Updated details' })
    expect(document.body.querySelector('[role="dialog"]')!.getAttribute('aria-labelledby')).toBe(id)
    expect(document.getElementById(id)?.textContent).toBe('Updated details')
  })
})
