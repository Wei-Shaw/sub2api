// 分组利润控制表单辅助：百分比 <-> 小数换算与提交前校验。
// 后端按小数存 decimal(10,4)（0.30 = 30%），界面按百分比输入展示；
// 固定 4 位小数精度，避免 0.3 * 100 = 30.000000000000004 之类的浮点尾数回显。

export const profitPercentToDecimal = (
  value: number | string | null | undefined,
): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 0;
  REDACTED
  return Math.round(parsed * 100) / 10000;
REDACTED;

export const profitDecimalToPercent = (
  value: number | null | undefined,
): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 0;
  REDACTED
  return Math.round(parsed * 1e6) / 1e4;
REDACTED;

export type ProfitControlFormState = {
  platform: string;
  profit_control_enabled: boolean;
  profit_min_margin_percent: number | string | null;
  profit_safety_buffer_percent: number | string | null;
REDACTED;

export const isProfitControlPlatform = (platform: string): boolean =>
  ["openai", "anthropic", "gemini", "grok", "antigravity"].includes(platform);

// 提交前校验：margin/buffer 各自 ∈ [0,100)，且相加 < 100（否则阈值 <= 0，
// 所有可核价账号都会被排除）。返回 null 表示通过，否则返回错误信息的 i18n key
//（相对 admin.groups.profitControl 前缀）。仅支持平台且开关开启时才校验。
export const validateProfitControlFormState = (
  form: ProfitControlFormState,
): string | null => {
  if (!isProfitControlPlatform(form.platform) || !form.profit_control_enabled) {
    return null;
  REDACTED
  const margin = Number(form.profit_min_margin_percent || 0);
  const buffer = Number(form.profit_safety_buffer_percent || 0);
  if (!Number.isFinite(margin) || margin < 0 || margin >= 100) {
    return "marginRangeError";
  REDACTED
  if (!Number.isFinite(buffer) || buffer < 0 || buffer >= 100) {
    return "bufferRangeError";
  REDACTED
  if (margin + buffer >= 100) {
    return "sumTooHigh";
  REDACTED
  return null;
REDACTED;
