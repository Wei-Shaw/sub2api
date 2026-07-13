package domain

// SupportTicketImage 是工单/回复附带的图片描述。
//
// 之所以单独放在 domain 包（而不是 service）：
//   - ent schema 的 field.JSON 需要传入一个具体类型作为值的"形状"。
//   - 直接在 ent/schema 包里定义会触发 ent 生成的 mutation.go 反向 import，
//     形成 schema → mixins → intercept → ent → schema 的循环（与 RechargePromoTier 同理）。
//   - service / dto / handler 各层复用同一份内存结构，序列化字段名保持稳定。
//
// 生命周期：
//   - 上传阶段：POST /api/v1/support/tickets/attachments 校验图片、写 COS、返回一条记录。
//   - 提交阶段：CreateTicket / AppendReply 收到 []SupportTicketImage 后落库到 jsonb 列。
//   - 展示阶段：前端直接使用 URL 展示（bucket 公开可读，无需签名）。
//
// URL 只允许指向已配置的 COSImageConfig 对应的公开域名，service 层做前缀白名单校验，
// 防止调用方伪造外链或把任意 URL 塞进工单造成 SSRF/钓鱼风险。
type SupportTicketImage struct {
	// Key 是对象存储里的 object key（含 support-tickets/ 前缀），
	// 保留下来便于日后清理孤儿对象或做审计。
	Key string `json:"key"`
	// URL 是对外可展示的完整 URL（含 scheme + host + path）。
	URL string `json:"url"`
	// Size 是原始字节数（用于前端限额提示 / 后端二次校验，不用于计费）。
	Size int64 `json:"size"`
	// MIME 是规范化后的 image/* 类型（image/png 或 image/jpeg）。
	MIME string `json:"mime"`
}
