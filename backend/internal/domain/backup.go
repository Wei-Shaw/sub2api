package domain

// BackupS3Config S3 兼容存储配置（支持 Cloudflare R2）
type BackupS3Config struct {
	Endpoint        string `json:"endpoint"` // e.g. https://<account_id>.r2.cloudflarestorage.com
	Region          string `json:"region"`   // R2 用 "auto"
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key,omitempty"` //nolint:revive // field name follows AWS convention
	Prefix          string `json:"prefix"`                      // S3 key 前缀，如 "backups/"
	ForcePathStyle  bool   `json:"force_path_style"`
}

// IsConfigured 检查必要字段是否已配置
func (c *BackupS3Config) IsConfigured() bool {
	return c.Bucket != "" && c.AccessKeyID != "" && c.SecretAccessKey != ""
}
