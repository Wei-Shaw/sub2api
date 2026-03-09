export const TOKENS_PER_MILLION = 1_000_000

interface TokenPriceFormatOptions {
  fractionDigits?: number
  withCurrencySymbol?: boolean
  emptyValue?: string
REDACTED

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
REDACTED

export function calculateTokenUnitPrice(
  cost: number | null | undefined,
  tokens: number | null | undefined
): number | null {
  if (!isFiniteNumber(cost) || !isFiniteNumber(tokens) || tokens <= 0) {
    return null
  REDACTED

  return cost / tokens
REDACTED

export function calculateTokenPricePerMillion(
  cost: number | null | undefined,
  tokens: number | null | undefined
): number | null {
  const unitPrice = calculateTokenUnitPrice(cost, tokens)
  if (unitPrice == null) {
    return null
  REDACTED

  return unitPrice * TOKENS_PER_MILLION
REDACTED

export function formatTokenPricePerMillion(
  cost: number | null | undefined,
  tokens: number | null | undefined,
  options: TokenPriceFormatOptions = {REDACTED
): string {
  const pricePerMillion = calculateTokenPricePerMillion(cost, tokens)
  if (pricePerMillion == null) {
    return options.emptyValue ?? '-'
  REDACTED

  const fractionDigits = options.fractionDigits ?? 4
  const formatted = pricePerMillion.toFixed(fractionDigits)
  return options.withCurrencySymbol == false ? formatted : `$${formattedREDACTED`
REDACTED
