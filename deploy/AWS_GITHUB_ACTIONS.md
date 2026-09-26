# GitHub Actions to AWS production deployment

Pushes to `custom-ui` build an immutable GHCR image and deploy that exact commit to the production EC2 instance through AWS Systems Manager Run Command.

## Security boundary

- GitHub obtains short-lived AWS credentials through OIDC. No AWS access key is stored in GitHub.
- The IAM trust policy only accepts `DomenLee/gptplusch-sub2api` on `refs/heads/custom-ui`.
- The workflow clears inherited credentials and verifies the STS caller is the expected assumed role with a non-empty session token before sending an SSM command.
- The deployment role can only send `AWS-RunShellScript` commands to the production instance and read the resulting command status.
- The instance remains reachable through SSM; public SSH is not required.

## Production behavior

The deployment script:

1. serializes releases with `flock`;
2. rejects dirty tracked files and unexpected image names;
3. records the old commit and image;
4. creates a PostgreSQL custom-format dump and backs up both Compose files;
5. fast-forwards the server checkout to the workflow commit;
6. pins `docker-compose.prod.yml` to the immutable commit image;
7. recreates only `sub2api`, leaving PostgreSQL and Redis running;
8. verifies container health, public version output, database readiness, and startup logs;
9. restores the previous image and Compose override when a post-switch check fails.

Backups are stored under `deploy/release-backups/` on the production server. Database restoration is intentionally manual because it is destructive.

## AWS resources

- Region: `ap-northeast-1`
- Instance: `i-0e54fc7a3f3d399d5`
- Deployment role: `arn:aws:iam::386566550227:role/gptplusch-sub2api-github-deploy`
- OIDC provider: `arn:aws:iam::386566550227:oidc-provider/token.actions.githubusercontent.com`

The resources are managed by `deploy/aws-github-actions-role.yml`. Apply changes with:

```bash
aws cloudformation deploy \
  --stack-name gptplusch-sub2api-github-deploy \
  --template-file deploy/aws-github-actions-role.yml \
  --capabilities CAPABILITY_NAMED_IAM \
  --region ap-northeast-1 \
  --profile sub2api
```

## Manual verification

```bash
aws ssm describe-instance-information \
  --region ap-northeast-1 \
  --filters Key=InstanceIds,Values=i-0e54fc7a3f3d399d5

ssh sub2api \
  "docker inspect sub2api --format '{{.Config.Image}} {{if .State.Health}}{{.State.Health.Status}}{{end}}'"
```
