# Web3 Deposit Operations and Rollout Runbook

This runbook covers the Conflux eSpace USDT0 deposit pipeline. It assumes one shared EVM deposit wallet, exact decimal accounting, finalized-only credit eligibility, and independently controlled user entry, scanning, and crediting.

> **Asset-model constraint:** Web3 deposits currently support only product- and operations-approved USD stablecoins, credited at a fixed `1 Token = 1 USD` into the internal `usdt` Web3 balance. There is no price oracle, FX conversion, or depeg handling. The supported and validated topology is multiple configured networks with one deposit token per network. The `assets` map preserves schema extensibility; it does not mean multiple tokens on the same network are supported. A non-USD asset, volatile token, or second token on one network requires a separate design, Spec, implementation, and acceptance test before rollout.

## Safety invariants

- Never configure a private key or seed phrase in the application.
- Never paste the account xpub into tickets, logs, chat, dashboards, or screenshots.
- Keep `wallet_id` bound to the original account path and xpub fingerprint for its lifetime.
- Never edit `web3_deposits` chain-fact fields such as amount, user, token, transaction hash, log index, block hash, or finalized time.
- Never update Scanner or Finalizer cursors manually to perform a rescan.
- Disable user entry before disabling observation or crediting during an incident.
- Do not configure a non-USD stablecoin or more than one deposit token for an enabled network under the current asset model.

## Runtime controls

The three rollout switches are independent:

| Setting | Effect |
| --- | --- |
| `WEB3_DEPOSIT_ENABLED` | Master switch required by all other Web3 deposit functions. |
| `WEB3_DEPOSIT_SCANNER_ENABLED` | Enables RPC scanning and finalization. |
| `WEB3_DEPOSIT_CREDIT_ENABLED` | Enables the Credit Worker that moves finalized deposits into the Web3 balance. |
| `WEB3_DEPOSIT_USER_ENTRY_ENABLED` | Exposes the user navigation and permits new address allocation. |

Operational shutdown order:

1. Set `WEB3_DEPOSIT_USER_ENTRY_ENABLED=false`.
2. Set `WEB3_DEPOSIT_CREDIT_ENABLED=false` if balance crediting must stop.
3. Set `WEB3_DEPOSIT_SCANNER_ENABLED=false` only when chain observation must stop.
4. Keep the database and existing deposit facts intact.

## Health and metrics

Use the administrator Web3 Deposit console or `GET /api/v1/admin/web3-deposits/runtime`.

The response contains one `runtimes` entry for every enabled `network_key + asset_key` pair. Monitor each entry independently:

- `network_key` and `asset_key` identify the runtime and its persisted Scanner cursor.
- `state` and `leader`: exactly one healthy Scanner leader is expected per pair.
- `latest_block`, `scanned_block`, and `lag_blocks` per pair.
- `endpoints[].healthy` and `endpoints[].unhealthy_until` for the pair-specific RPC pool.
- `metrics.rpc_healthy`.
- `metrics.scanner_lag_blocks` and `metrics.finalizer_lag_blocks`.
- `metrics.scanner_failures_total` and `metrics.finalizer_failures_total`.
- `metrics.orphaned_deposits_total`.
- `metrics.credit_retries_total` and `metrics.credit_failures_total`.
- `status_counts.manual_review`, `status_counts.failed`, `status_counts.ready_to_credit`, and `status_counts.crediting`.

Investigate sustained lag, continuously increasing failures, or deposits stuck beyond the configured polling and lease intervals.

## RPC endpoint switch

1. Confirm the replacement endpoint reports Conflux eSpace chain ID `1030`.
2. Confirm the official USDT0 contract exists and reports decimals `6`.
3. Confirm the endpoint supports the `finalized` block tag, receipts, block lookup, and bounded `eth_getLogs`.
4. Add the endpoint to `WEB3_DEPOSIT_NETWORKS_CONFLUX_ESPACE_MAINNET_RPC_URLS`; keep at least one known-good fallback during the change.
5. Restart one application instance and confirm `web3_deposit_rpc_ready` appears without credentials in the log.
6. Confirm Scanner lag decreases and no canonical mismatch surge occurs.
7. Roll the remaining instances.
8. Remove the failed endpoint only after the replacement has remained healthy.

If verification fails, the runtime fails closed and must not allocate new addresses or scan through an unverified endpoint.

## Stop and resume Scanner

To stop scanning while preserving facts and cursors:

1. Set `WEB3_DEPOSIT_SCANNER_ENABLED=false` and restart instances normally.
2. Confirm every runtime state becomes `disabled` or `stopped` and no instance holds a Scanner lease.
3. Do not alter `web3_scanner_cursors`.
4. Resolve the incident or RPC configuration.
5. Re-enable the Scanner and confirm it resumes from its persisted cursor with the configured overlap.

Duplicate log reads after restart are expected and safe because the event identity is `(chain_id, tx_hash, log_index)`.

## Bounded rescan

Use the administrator console's **Bounded rescan** action.

1. Select the exact `network_key + asset_key` runtime that owns the chain and token contract.
2. Determine the exact inclusive start and end block from chain evidence.
3. Keep the range within that network's configured block batch size.
4. Complete step-up verification.
5. Submit the target and range, then record the returned event, matched, and deposit counts.
6. Check the deposit list for newly detected events.
7. Confirm the production Scanner cursor for the selected pair did not change as a result of the operation.

Run adjacent non-overlapping ranges for a larger recovery window. Repeating the same range is safe and does not create duplicate event facts.

## Manual review

### Approve

1. Open a `manual_review` deposit and verify user, destination address, amount, token contract, transaction hash, and review reason.
2. Confirm the transaction on an independent Conflux eSpace explorer or RPC source when practical.
3. Select **Approve** and complete step-up verification.
4. The server rechecks the finalized height, canonical block hash, receipt status, receipt block hash, Transfer log, destination, token, and raw amount.
5. A successful approval moves only the status to `ready_to_credit`; the Credit Worker performs the idempotent balance transaction.
6. Confirm the deposit becomes `credited` and the Web3 balance increases exactly once.

### Ignore

1. Select **Ignore** only for a deposit that must not be credited.
2. Enter a specific operational reason; blank reasons are rejected.
3. Complete step-up verification.
4. Confirm the status becomes `ignored` and no balance changes occur.

### Retry failed credit

1. Resolve the root cause before retrying.
2. Retry only a `failed` deposit through the administrator console.
3. Complete step-up verification.
4. Confirm it returns to `ready_to_credit` and is claimed by the Credit Worker.
5. Verify the deposit status, Web3 balance, transfer facts, and user balance before attempting another retry.

All approve, ignore, retry, and rescan requests are protected by administrator authentication, step-up verification, and the existing administrator audit log middleware.

## Wallet recovery and fingerprint verification

1. Recover the dedicated wallet offline from the original seed backup.
2. Derive the exact configured account path, normally the stored account-level path for `wallet_id=evm_deposit_v1`.
3. Export only the account-level xpub to the secret-management workflow; never export a private key.
4. Compute the fingerprint using the same canonical xpub normalization and SHA-256 procedure used by the application.
5. Compare it with `web3_deposit_wallets.xpub_fingerprint` and confirm the account path matches.
6. Derive several known address indexes offline and compare them with existing `web3_deposit_addresses` records.
7. Only after all checks match, restore the account xpub configuration and restart one instance.
8. Confirm wallet startup verification succeeds before enabling user entry.

A fingerprint or account-path mismatch is a stop condition. Do not replace the xpub under an existing wallet ID; create and migrate through a separately designed wallet-rotation procedure.

## Production rollout

### 1. Database migration

- Apply migrations before enabling any Web3 feature switch.
- Verify all Web3 tables, primary keys, unique event identity, balance constraints, and indexes.
- Keep all four switches disabled.

### 2. Observe-only Scanner

- Configure the verified account xpub, path, RPC endpoints, chain ID, Token contract, decimals, scan start block, and limits.
- Enable the master switch and Scanner only.
- Keep `CREDIT_ENABLED=false` and `USER_ENTRY_ENABLED=false`.
- Verify RPC health, leader election, Scanner lag, Finalizer lag, duplicate-scan behavior, and canonical validation.

### 3. Real small-value validation

- Allocate or select an administrator-owned gray account address through a controlled process.
- Send a small amount above the minimum using the official USDT0 contract on Conflux eSpace.
- Verify the chain event, detected record, finalized record, exact decimal conversion, and expected `ready_to_credit` state.

### 4. Administrator gray crediting

- Enable `CREDIT_ENABLED=true` while keeping the public user entry disabled.
- Verify exactly-once credit into `web3_user_balances`.
- Perform a controlled transfer to the main user balance and reconcile the immutable transfer fact, `users.balance`, and `total_recharged`.
- Verify cache invalidation and notification behavior.

### 5. User entry

- Enable `USER_ENTRY_ENABLED=true` only after database reconciliation and operator sign-off.
- Confirm the user navigation, address query/create flow, QR code, contract warning, history, and status mapping.
- Monitor lag, retries, failures, manual-review backlog, address-allocation errors, and support reports closely during the initial window.

## Rollback

1. Disable user entry immediately.
2. Disable crediting if accounting correctness is uncertain.
3. Leave scanning enabled when possible so chain facts continue to be recorded.
4. Disable scanning only for RPC, canonical-verification, or data-ingestion incidents.
5. Do not delete deposits, balance facts, transfers, addresses, wallets, or cursors.
6. Reconcile every `ready_to_credit`, `crediting`, `failed`, and `manual_review` record before re-enabling crediting.

## Reconciliation checklist

For each credited deposit confirm:

- One unique `web3_deposits` event fact exists.
- The deposit reached `credited` only after finalized canonical verification.
- `web3_user_balances.total_deposited` increased by the credited amount exactly once.
- Any transfer to the main balance has one unique `web3_balance_transfers` record.
- `available_amount`, `total_transferred`, `users.balance`, and `users.total_recharged` reconcile exactly using decimal arithmetic.
- No PaymentOrder, RedeemCode, or affiliate ledger record was created by the Web3 deposit path.
