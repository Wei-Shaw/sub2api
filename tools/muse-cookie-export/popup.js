// Adapted from czg86389-hub/muse2api (MIT); see THIRD_PARTY_NOTICES_MUSE.md.
// HttpOnly cookies require chrome.cookies; document.cookie cannot read them.
const names = ['hatch_sess', 'hatch_gw', 'hatch_vml', 'hatch_native_auth_device']
const button = document.getElementById('export')
const status = document.getElementById('status')
button.addEventListener('click', async () => {
  button.disabled = true
  status.textContent = ''
  try {
    const all = await chrome.cookies.getAll({ url: 'https://muse.ai/' })
    const cookies = {}
    const expires = {}
    for (const cookie of all) {
      if (cookie.domain.replace(/^\./, '').toLowerCase() !== 'muse.ai' || cookie.path !== '/' || !names.includes(cookie.name)) continue
      if (cookies[cookie.name] && cookies[cookie.name] !== cookie.value) throw new Error('ambiguous')
      cookies[cookie.name] = cookie.value
      if (cookie.expirationDate) expires[cookie.name] = Math.floor(cookie.expirationDate)
    }
    const missing = names.filter(name => name !== 'hatch_vml' && !cookies[name])
    if (missing.length) {
      status.textContent = `Sign in to Muse first. Missing: ${missing.join(', ')}`
      return
    }
    const downloadUrl = 'data:application/json;charset=utf-8,' + encodeURIComponent(JSON.stringify({ cookies, expires }, null, 2))
    await chrome.downloads.download({ url: downloadUrl, filename: 'muse-session.json', saveAs: true })
    status.textContent = 'Exported your session credential file. Import it into your Sub2API Muse account.'
  } catch {
    status.textContent = 'Session export failed. Check that you are signed in to Muse.'
  } finally {
    button.disabled = false
  }
})
