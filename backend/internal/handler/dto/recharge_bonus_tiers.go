package dto

// RechargeBonusTier 充值赠送档位：余额充值支付金额 ≥ MinAmount 时，在到账基数上赠送 BonusPercent%。
type RechargeBonusTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}
