package service

// InsufficientBalanceClientMessage is the bilingual, actionable message used
// when a request is rejected because the user's wallet balance is exhausted.
// Keep the frontend route relative so the message remains valid behind a
// reverse proxy or a custom frontend domain.
const InsufficientBalanceClientMessage = "Insufficient account balance. Please visit the store to purchase a redemption code and redeem it for balance, then retry. / 账户余额不足，请前往商店购买卡密兑换充值额度后重试。"
