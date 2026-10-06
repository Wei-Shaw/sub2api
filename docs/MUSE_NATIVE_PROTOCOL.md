# Consumer Muse protocol qualification

Tracking: [#7625](https://github.com/Wei-Shaw/sub2api/issues/7625),
[draft PR #7639](https://github.com/Wei-Shaw/sub2api/pull/7639).

The 2026-10-06 capture used an authorized consumer `muse.ai` account. Its General
settings showed **Free**, with additional tokens available. It does not establish
paid subscription access. Credentials, account/VM identifiers, OTPs, and raw
captures remain outside this repository.

## Observed and reproduced natively

1. Phone sign-in obtains three persistent HttpOnly credentials. Native Go
   `GET /api/session` obtains the fourth VM-lease cookie and the assigned VM.
   The host-only lease deletion and domain replacement must be processed together.
2. `POST /api/hatch/token` accepts `{vmAddress, vmName}` and returns the gateway
   credential. The browser's same-origin request metadata was necessary in the
   native probe: the earlier request returned 403 despite successful session
   renewal. Adding another device cookie did not resolve that rejection.
3. The shared WebSocket endpoint is `wss://hatch.metaaivm.com/v1/noise`, with
   VM ID, auth token, app ID, request ID, and an optional notary token. Muse
   cookies are sent only to `muse.ai`; they are not copied to the gateway.
4. The handshake is `Noise_XX_25519_AESGCM_SHA256`. Message 1 carries a fresh
   32-byte nonce. The observed standard peer statement binds that nonce and the
   server static key. The native handshake reproduced this binding and completed
   over ordinary certificate-validated TLS. This is **not hardware attestation**.
5. `GET /model` succeeded over the native encrypted channel. The observed catalog
   contained `auto/auto` and `ipnext/avocado-5.16-v0`. These are dated observations,
   not a hardcoded production catalog or a guarantee for other accounts.
6. A fresh `POST /chat/stream` side chat accepted a synthetic text-only probe.
   Its acknowledgement contained the requested session ID, server message ID,
   matching reply ID, and thread/channel metadata. Subsequent native history
   retrieval found that same side chat's completed probe response after the
   submitting connection closed. No prompt was replayed.
7. Output/recovery uses a separate `POST /chat/subscribe` subscription, with
   `session_id`, replay cursors, and capabilities. Replay for another accepted
   probe returned its completed `delta.message_done` transcript and the same
   side-chat agent's completed task status with zero active subagents.
8. A bounded native counting probe produced 126 live text-delta records. Text
   deltas omit `agent_id` and carry the output message ID and `parent_agent_id`;
   the corresponding message-start record supplies the agent binding.
9. Another bounded probe was cancelled after a live delta. `POST /chat/cancel`
   acknowledged `cancelled: true`; the same running task then reached
   **`interrupted`** with zero active subagents, in the same side chat/root agent.
   A cancellation acknowledgement alone was not used as terminal proof.

The native low-level implementation is in `internal/pkg/muse/gateway.go` and
`noise_*.go`. It uses the established Noise library rather than implementing
cryptographic primitives. No downloaded Muse client code is executed or embedded.
The private qualification executable was temporary; it is not a service sidecar.

## Framing and subscription contracts

The consumer client describes an encrypted protobuf chunk frame containing a
signed 64-bit chunk ID, chunk index, total chunks, and payload. A reassembled
service response contains a stream envelope: stream ID and one response, body
chunk, or reset. HTTP responses carry status, headers, body, and an end-body flag.

The implementation preserves observed limits of 65,535 encrypted bytes per frame,
65,489 payload bytes per chunk, and 256 chunks. It bounds incomplete assemblies,
aggregate retained bytes, and assembly lifetime. Authentication/framing failures
poison the channel; it never retries with a possibly desynchronized nonce.

Subscription records are bounded NDJSON and can span encrypted body chunks. The
initial acknowledgement can be a standalone JSON object or a `res`/`ok` RPC
wrapper. A body ending or a transport reset does not establish task completion.
Likewise, the observed agent activity code `online` is not proof that a task is
terminal. Task status and the completed transcript must be correlated to the
operation's authenticated VM, side chat, and subscribed agent.

Standard statements and confidential owner-RV challenges share protobuf field
numbers. Classification must inspect the complete shape. The observed account
used the standard statement; its browser had no unlocked recovery key. The native
implementation rejects owner-RV challenges and delegated notary credentials
until their owner-bound key, endorsement, and attestation flows are qualified.
It does not enroll a key, replace a VM, disable checks, or approve Sentinel actions.

## Remaining provider work

Production wiring still uses `muse.DisabledProvider`. These transport observations
do not yet qualify public Sub2API inference. The remaining implementation and live
checks are:

- Authenticated principal/session binding, fresh allowance, and dynamic catalog
  observation through the native provider verification port.
- Authoritative transcript/delta normalization into existing Responses,
  Chat Completions, and Messages serializers, preserving unknown usage.
- Durable mapping from a local operation to its fresh remote side chat and server
  task, including lost-ack recovery without replay, duplicate events, proactive
  event exclusion, and accepted-work fencing.
- Integrating confirmed cancellation of the same task, approval-required and remote failures,
  session expiry/rotation during active work, and process-loss reconciliation.
- The existing account/proxy snapshot, canonical lock, admission, settlement, and
  quota flow exercised through a real local Sub2API API request, with one result,
  one usage row, and one frozen flat charge after fault injection.
- Separate paid-account and confidential-VM qualification when those modes are
  supported. The present Free account must not be advertised as paid verification.

## Evidence sources and validation

Protocol field shapes were reviewed in the public assets actually loaded by the
authenticated app: `2kzwdkemdsxkx.js` (Noise and protobuf framing),
`3cd-x-0rbods0.js` (credential targets and token requests), `153vpyuy7g7ui.js`
(subscription acknowledgements/records), `3og77vozpfi04.js` (RPC routes), and
`3r93bf493vh-m.js` (submission and replay). Asset names are observations from this
client build, not stable API version identifiers. The browser UI sign-in was
observed; the initial network buffer overflowed, so its OTP HTTP trace is partial.
Later native/probe captures are separate evidence.

Cryptographic behavior is defined by the
[Noise specification](https://noiseprotocol.org/noise.html) and
[flynn/noise v1.1.0](https://pkg.go.dev/github.com/flynn/noise@v1.1.0).
Unit/race tests cover target restrictions, proxy/request context, credential-safe
errors, peer binding, owner-RV rejection, multiplexing, malformed frames, bounded
assembly, record fragmentation, RPC wrappers, and mismatched acceptance.
Envelope fuzzing is bounded. Automated tests use synthetic credentials and do not
contact Meta. Live qualification remains an explicit, private operator activity.
