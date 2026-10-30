import { describe, expect, it } from 'vitest'
import { getAdminSteps, getUserSteps } from '../steps'

const t = (key: string) => (key === 'common.userMenu' ? 'User "menu"' : `<${key}>`)

describe('onboarding steps', () => {
  it('escapes translated text before rendering it as HTML', () => {
    const [welcome] = getAdminSteps(t)
    const html = welcome.popover?.description as string
    expect(html).toContain('&lt;onboarding.admin.welcome.intro&gt;')
    expect(html).not.toContain('<onboarding.admin.welcome.intro>')
  })

  it('only targets navigation entry points, never forms or submit buttons', () => {
    const selectors = [...getAdminSteps(t), ...getUserSteps(t)]
      .map((step) => step.element)
      .filter((element): element is string => typeof element === 'string')

    expect(selectors.length).toBeGreaterThan(0)
    for (const selector of selectors) {
      expect(selector).toMatch(/^(aside\.sidebar a\[href=|header button\[aria-label=)/)
      expect(selector).not.toMatch(/form|submit|create-btn/)
    }
  })

  it('locates the user menu by its translated aria-label', () => {
    const last = getAdminSteps(t).at(-1)
    expect(last?.element).toBe('header button[aria-label="User \\"menu\\""]')
  })

  it('starts each tour with a centered welcome step', () => {
    expect(getAdminSteps(t)[0].element).toBeUndefined()
    expect(getUserSteps(t)[0].element).toBeUndefined()
  })
})
