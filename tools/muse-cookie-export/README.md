# Muse Connect browser helper

Connect the Muse account already signed in in Chrome/Edge directly to your
Sub2API account form. No session JSON file is created or moved.

## Connect

1. Install the helper once: unzip the download, open `chrome://extensions` or
   `edge://extensions`, enable Developer mode and choose **Load unpacked** for
   the `muse-cookie-export` folder.
2. Sign in to `https://muse.ai/` in the same browser.
3. In your HTTPS Sub2API account form, select the owning user and click
   **Connect Muse**. Open **Sub2API Muse Connect** from the browser toolbar.
4. Check the displayed Sub2API origin and assigned user, then click
   **Approve connection**. The session goes directly to that pending form.
5. Save the account. Sub2API automatically verifies native Muse model access.
   A saved account stays saved if verification needs to be retried.

The pending request expires after two minutes and is bound to the tab, exact
origin, owner, account and a single-use 256-bit nonce. Changing or closing the
form cancels it. Approvals are never sent to arbitrary URLs or all frames.

## Permissions and privacy

- `cookies` with host access restricted to `https://muse.ai/*`, for the four
  named Muse sign-in cookies. Cookies are read only after approval.
- `activeTab` and `scripting`, for the Sub2API tab where you explicitly invoke
  the helper. No permanent access to every website is requested.
- No network fetch from the helper, download permission, credential URL,
  local/session storage, analytics or credential logging. The authenticated
  Sub2API form uses its normal save API after your approval.

Public HTTP destinations are rejected. HTTP localhost/loopback is allowed for
development. File import and JSON paste remain available under **Advanced** in
Sub2API for existing exports; this helper does not export session files.

Cookie handling is adapted from MIT-licensed reference Muse integrations; see
`THIRD_PARTY_NOTICES_MUSE.md`. Native inference, entitlement and reported usage
still require their separate provider checks; a helper handoff is not an
inference or subscription claim.

After changing runtime files, rebuild the bundled download from the repository
root with `python3 tools/muse-cookie-export/package.py`.
