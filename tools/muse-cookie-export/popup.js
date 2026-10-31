// Adapted cookie selection from czg86389-hub/muse2api (MIT); see THIRD_PARTY_NOTICES_MUSE.md.
import { readPending, connectSession } from './connect.mjs'
const button = document.getElementById('connect')
const status = document.getElementById('status')
let approved
try {
  approved = await readPending(chrome)
  document.getElementById('origin').textContent = approved.request.origin
  document.getElementById('owner').textContent = approved.request.ownerLabel
  document.getElementById('destination').hidden = false
  document.getElementById('instructions').textContent = 'Approve only if you trust this destination. Your session stays in this form until you save the account.'
  button.disabled = false
} catch {
  status.textContent = 'Start a new connection in the Sub2API form, then open this helper from that same tab. HTTPS is required (localhost is allowed for development).'
}
button.addEventListener('click', async () => {
  if (!approved) return
  button.disabled = true
  status.textContent = 'Connecting…'
  try {
    await connectSession(chrome, approved)
    approved = undefined
    status.textContent = 'Session sent. Return to Sub2API and save the account to finish verification. No session file was created.'
  } catch (error) {
    approved = undefined
    status.textContent = ['sign-in-required', 'expired-session'].includes(error?.message)
      ? 'Sign in to Muse, return to the Sub2API tab and start Connect Muse again.'
      : 'The connection expired or changed. Start Connect Muse again in the intended Sub2API form.'
  }
})
