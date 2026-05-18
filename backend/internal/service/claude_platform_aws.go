package service

import (
	"fmt"
	"strings"
)

const claudePlatformAWSAuthMode = "claude_platform_aws"

func claudePlatformAWSRegion(account *Account) string {
	if account == nil {
		return defaultBedrockRegion
	}
	if region := strings.TrimSpace(account.GetCredential("aws_region")); region != "" {
		return region
	}
	return defaultBedrockRegion
}

func claudePlatformAWSWorkspaceID(account *Account) string {
	if account == nil {
		return ""
	}
	if workspaceID := strings.TrimSpace(account.GetCredential("workspace_id")); workspaceID != "" {
		return workspaceID
	}
	return strings.TrimSpace(account.GetCredential("anthropic_workspace_id"))
}

func BuildClaudePlatformAWSBaseURL(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		region = defaultBedrockRegion
	}
	return fmt.Sprintf("https://aws-external-anthropic.%s.api.aws", region)
}

func BuildClaudePlatformAWSMessagesURL(region string) string {
	return strings.TrimRight(BuildClaudePlatformAWSBaseURL(region), "/") + "/v1/messages?beta=true"
}

func BuildClaudePlatformAWSCountTokensURL(region string) string {
	return strings.TrimRight(BuildClaudePlatformAWSBaseURL(region), "/") + "/v1/messages/count_tokens?beta=true"
}
