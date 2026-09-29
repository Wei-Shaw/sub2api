# Muse session exporter

This Chromium extension exports the four Muse app HttpOnly cookies used by the
reference muse2api projects. It saves a local JSON credential file and sends
nothing to another server. Its source and MIT attribution are in
`THIRD_PARTY_NOTICES_MUSE.md`.

1. Open Chrome/Edge extension management, enable developer mode, and choose
   **Load unpacked** with this directory.
2. Sign in to `https://muse.ai/` using the dedicated account assigned to one
   Sub2API user.
3. Open the extension and choose **Export session JSON**.
4. Paste that file's JSON into the native Muse account session field. CDP and
   Playwright cookie-array exports are also accepted.
5. In the account editor, choose **Check app cookies**. Sub2API uses the account's
   configured proxy and updates returned cookies atomically. It does not wake a VM
   or send a prompt during this check.

Session authentication does not qualify inference, models, or subscription quota.
Those remain separate verification requirements. The native inference provider
continues to be disabled until its app transport is qualified.
