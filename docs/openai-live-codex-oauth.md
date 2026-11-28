# Experimental Codex OAuth Live

Sub2API's default Live authentication mode, `device_check`, uses the existing
desktop DeviceCheck provider. The optional `codex_oauth` mode implements the
OAuth signaling flow emitted by the native Codex client. It does not generate
or impersonate a desktop device proof. The upstream service still authenticates
the account and decides whether that account may create a Live session.

## Enable the experimental mode

Set the server configuration and restart the application:

```yaml
gateway:
  live:
    upstream_auth_mode: codex_oauth
    max_session_duration_seconds: 3600
```

The equivalent environment variable is
`GATEWAY_LIVE_UPSTREAM_AUTH_MODE=codex_oauth`. Omitting the setting retains
`device_check`. Unknown values are rejected at startup.

Live must also be enabled for the API key's OpenAI group. An eligible OpenAI
OAuth account is required; this mode does not add Live support to API-key
upstream accounts. Browser voice availability alone does not establish that
the account can use the Codex Live endpoint.

Configure the Codex custom provider's base URL with `/v1`, for example:

```toml
# Top-level setting, before any [table]. Required for Codex CLI 0.166.0's
# custom-provider sideband; otherwise it defaults to api.openai.com.
experimental_realtime_ws_base_url = "ws://127.0.0.1:8080/v1/live"

[model_providers.example]
name = "Local Sub2API"
base_url = "http://127.0.0.1:8080/v1"
wire_api = "responses"
```

Keep the existing provider's authentication settings. The native client's
`/live` request then reaches Sub2API's `/v1/live` route. A base URL without
`/v1` may reach the web frontend instead of the Live API. Both the HTTP creation
and WebSocket sideband must use Sub2API; setting the provider's HTTP base alone
does not redirect the sideband in CLI 0.166.0. For a remote HTTPS deployment,
use its corresponding `wss://` endpoint instead of this loopback example.

## WSL2 audio devices

The Linux Codex CLI also needs a local audio device. WSLg exposes Windows audio
through PulseAudio, normally at `unix:/mnt/wslg/PulseServer`. On Ubuntu, the
ALSA-to-PulseAudio runtime and inspection utility are provided by
`libasound2-plugins`, `libpulse0`, and `pulseaudio-utils`. Installing these
packages does not require a separate PulseAudio daemon inside Ubuntu.

Check the WSLg connection without recording or playing audio:

```sh
test -S /mnt/wslg/PulseServer
pactl list short sinks
pactl list short sources
```

`RDPSink` and `RDPSource` are the WSLg output and input; `SUSPENDED` is normal
when no audio stream is active. In Codex CLI 0.166.0, open `/voice settings`,
choose `Set sound devices`, and select an available `Input device` and
`Output device`. The stock ALSA Pulse endpoint normally appears as
`PulseAudio Sound Server`.

Codex matches the device's displayed name, not its ALSA PCM ID. Do not set
`audio.microphone` or `audio.speaker` to the internal ID `pulse` unless that is
also the displayed name. These preferences belong under `audio`, not
`realtime.audio`. Selecting devices in Codex does not change other programs'
defaults.

Some ALSA Pulse plugin versions incorrectly report a fatal I/O error when a
new output stream has no timing information yet. If devices can be listed but
voice stops immediately, check whether the distribution includes the upstream
[Pulse latency error fix](https://github.com/alsa-project/alsa-plugins/commit/811c2ce69d1265ace42a05ef045ca1b4ed72e01c).
This local audio issue is independent of the gateway authentication mode.

Device enumeration only establishes availability; Windows microphone
permissions, the selected physical microphone, audible playback, and an actual
spoken exchange still require a user audio test.

## Compatibility and lifecycle

Each created call records its authentication mode and upstream session ID in
Redis. Its observer and sideband use the same recorded mode and session ID,
including after a configuration change. Old records without these fields
retain the DeviceCheck flow. Unknown stored modes are rejected.

The upstream routes differ by authentication mode. Both modes create the call
through ChatGPT's Codex Live endpoint. For the sideband, `codex_oauth` follows
native Codex CLI 0.166.0 and connects to
`wss://api.openai.com/v1/live/<call-id>` using the same subscription OAuth token;
it does not use a separately billed API key. `device_check` retains
`wss://chatgpt.com/backend-api/codex/<call-id>`. The recorded mode selects this
route for both observers and client reconnects, independently of later config
changes. See the official client's
[sideband provider selection](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/core/src/realtime_conversation.rs#L1379)
and [API-provider base URL](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/model-provider-info/src/lib.rs#L468).

Existing group permissions, account eligibility, API-key ownership, concurrency
leases, session duration limits, and usage finalization remain in effect.
The patch adds no PostgreSQL migration and does not change credentials.

The capability endpoint reports local support and the selected mode. For
`codex_oauth`, it also reports `experimental: true` and
`upstream_verified: false`: a local capability check does not create a call or
verify any account's entitlement.

## Validation and rollback

Validate session creation, the sideband, WebRTC media, and task handoff in an
isolated deployment before replacing a working service. Receiving HTTP 200 or
an SDP event alone does not establish a working audio session. Successful
upstream responses must contain a valid SDP envelope and a call Location.
Both `application/sdp` and `text/plain` (including a charset parameter) are
accepted for that envelope; the native endpoint has returned HTTP 201 with
`text/plain`. HTML and JSON error bodies remain invalid even with HTTP 2xx.

Stop active Live calls before rolling back the application binary. Restore
`device_check` or remove the setting to return to the legacy flow. Retain the
exact previous executable/image and configuration; an application updated
inside a container may be newer than that container's original image.
