## Change Type

- [ ] Feature
- [ ] Bug fix
- [ ] Security fix
- [ ] Operations/deployment
- [ ] Upstream synchronization
- [ ] Workflow/documentation
- [ ] Release preparation
- [ ] Hotfix

## Scope

Describe the intended outcome, non-goals, and user-visible behavior.

## Risk And Compatibility

Describe failure modes, backward compatibility, multi-instance/blue-green behavior, and sensitive-data impact.

## Database And Configuration

- Migrations:
- New or changed settings/environment variables:
- Old/new version coexistence:

## Verification

List exact local commands and GitHub Actions checks. State every skipped test and why it was skipped.

## Audit

- Audited commit SHA:
- Auditor/review method:
- Findings and disposition:
- Residual risks:

## Deployment And Rollback

Describe rollout, health signals, rollback target, and any condition that makes image-only rollback unsafe.

## Release Impact

- [ ] No release required
- [ ] Include in next planned release
- [ ] Requires immediate hotfix release
- [ ] This is a Release PR containing only VERSION and versioned release notes

## Checklist

- [ ] No secrets, production data, local reports, or duplicate ` 2` files are included
- [ ] Tests cover success, failure, and boundary behavior
- [ ] Required CI is green for the final commit SHA
- [ ] P0/P1 audit findings are resolved
- [ ] Documentation and release notes impact is addressed
- [ ] Upstream synchronization is isolated from feature development
