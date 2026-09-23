# Codex V2 Plaintext Collaboration Messages (Experimental)

**Status:** Experimental, opt-in. Off by default. Gateway integration is under
validation; real model calls have not been verified. This document describes
the account-option contract, not verified production behavior.

## Overview

Some Codex V2 multi-agent (collaboration) workflows attach work messages to
Responses requests in an encrypted form. When those sessions need to move
between accounts — for example when a pool rotates the upstream account — the
encrypted form can pin the session to the original account.

`accounts.extra.openai_responses_plaintext_collaboration` is a per-account
boolean that tells the gateway to emit *new* Codex V2 collaboration work
messages in plaintext form so they remain portable across accounts.

```json
{
  "extra": {
    "openai_responses_plaintext_collaboration": true
  }
}
```

Missing or `false` means the existing behavior is preserved: collaboration
messages pass through unchanged.

## Supported scope

- **Platform:** OpenAI accounts only.
- **Account types:** OAuth / Setup Token accounts and API Key accounts. The
  same key is used on both.
- **Traffic:** native Responses requests. The option is intended to apply to
  the Responses transports the gateway handles — HTTP, SSE streaming, and
  WebSocket (v2) sessions.
- **Unsupported outbound modes:** modes that do not forward native Responses
  payloads (for example `openai_responses_mode = force_chat_completions`).
  Enabling the option on such an account rejects requests (HTTP 400 or
  WebSocket close); it does not silently leave the option without effect.

The toggle is exposed in the admin UI on the account create, edit, and bulk
edit dialogs for eligible OpenAI accounts. Bulk edit leaves the setting
untouched unless its enable checkbox is explicitly selected.

Note the asymmetry: create/edit omit the key entirely when the option is off
(a missing key already means the same as `false`), while bulk edit writes an
explicit `false` when its enable checkbox is selected with the toggle off —
bulk partial-update semantics require the key to be present to clear it on
existing accounts.

## Pool consistency

Apply the same value to every OpenAI account in a pool. A collaboration
session created under plaintext on one account may not be readable by a pool
peer that still runs with the option off, so mixed settings can break session
migration mid-conversation.

## Limitations

- **Experimental.** Verify real tool calls against your own upstream before
  enabling; behavior is not yet validated across all upstream variants.
- **No retroactive recovery.** Enabling does not decrypt or repair previously
  encrypted collaboration history; it only affects new messages.
- **Namespace handling unchanged when off.** With the option off (default),
  Codex namespace tool declarations are preserved exactly as before; the
  related `openai_responses_flatten_namespaces` compatibility switch is
  unaffected.
- **Mode detection is per-account only.** The create/edit dialogs warn when
  the account is explicitly set to `openai_responses_mode =
  force_chat_completions`. Bulk edit cannot detect each selected account's
  Responses mode — do not enable the flag on accounts that force
  Chat Completions.

## Disabling

Turn the toggle off or remove the key from `extra`. New collaboration
messages then revert to the previous (encrypted pass-through) form. Sessions
already created in plaintext may not be readable once the upstream expects
encrypted messages again, so treat toggling mid-session as a boundary that
can strand existing history.
