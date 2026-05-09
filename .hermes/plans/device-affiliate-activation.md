# Device affiliate activation plan

## Scope clarified

Keep the existing V-Claw flow:

1. Client calls `/api/v1/vclaw/claim` with `claim_code`, `aff_code`, and device metadata.
2. Backend creates/returns a `DLG` device login code as today.
3. Client calls `/api/v1/auth/invite-login` with the `DLG`.
4. `invite-login` decides whether the device is allowed to use OpenClaw by checking `user_devices.status`.

Do not add a separate Device Activations UI. Reuse the existing admin user management UI and add enough status/filter/action support to identify and activate pending devices/users.

## Product behavior

- Affiliate code setting decides initial device status on first claim:
  - auto active: `user_devices.status = active`
  - manual active: `user_devices.status = pending_activation`
- Claim still returns the DLG in both cases.
- Invite-login with DLG:
  - `active` -> issue tokens/API keys as today.
  - `pending_activation` -> return a dedicated pending error/status; client must tell user to ask admin/marketing to activate.
  - `revoked`/`blocked` -> keep revoked/blocked behavior.
- Admin/marketing uses the existing user management screen:
  - show whether user/device is pending activation.
  - filter pending activation users/devices.
  - activate pending device/user from that UI.

## Implementation plan

### Backend

1. Add `UserDeviceStatusPendingActivation = "pending_activation"` and `ErrDeviceActivationPending`.
2. Add an affiliate setting field for device activation mode. Prefer extending `user_affiliates`/affiliate admin DTOs with values like `auto` and `manual`; default to `manual` for safety unless existing data should preserve current behavior.
3. In `VClawClaimService.createFirstClaim`, resolve the incoming `req.AffCode` policy and set `user_devices.status` accordingly instead of hardcoded `active`.
4. In `resumeExistingClaim`, return DLG and status for both `active` and `pending_activation`; only revoked/blocked should error.
5. In `completeDeviceInviteLogin`, explicitly block `pending_activation` with `DEVICE_ACTIVATION_PENDING` before token/API-key provisioning.
6. Extend user/admin listing DTO/repo queries to include device activation status summary and filter for pending activation.
7. Add admin action to activate a pending device/user via existing admin route surface.

### Frontend

1. Locate existing admin user management view and API client.
2. Add filter option for pending device activation.
3. Add visible status badge/column for device activation status.
4. Add activate action/button on pending rows, calling the backend activation endpoint.
5. Extend existing affiliate settings UI to configure device activation mode for an affiliate code.

### Verification

- Go format and targeted backend tests for claim/login/admin/affiliate behavior.
- Frontend lint/typecheck/build if practical.
- Manual diff review to ensure no new standalone Device Activations UI was introduced.
