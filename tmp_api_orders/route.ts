import { NextRequest, NextResponse REDACTED from 'next/server';
import { z REDACTED from 'zod';
import { createOrder, OrderError REDACTED from '@/lib/order/service';
import { getEnv REDACTED from '@/lib/config';

const createOrderSchema = z.object({
  user_id: z.number().int().positive(),
  amount: z.number().positive(),
  payment_type: z.enum(['alipay', 'wxpay']),
REDACTED);

export async function POST(request: NextRequest) {
  try {
    const env = getEnv();
    const body = await request.json();
    const parsed = createOrderSchema.safeParse(body);

    if (!parsed.success) {
      return NextResponse.json(
        { error: '参数错误', details: parsed.error.flatten().fieldErrors REDACTED,
        { status: 400 REDACTED,
      );
    REDACTED

    const { user_id, amount, payment_type REDACTED = parsed.data;

    // Validate amount range
    if (amount < env.MIN_RECHARGE_AMOUNT || amount > env.MAX_RECHARGE_AMOUNT) {
      return NextResponse.json(
        { error: `充值金额需在 ${env.MIN_RECHARGE_AMOUNTREDACTED - ${env.MAX_RECHARGE_AMOUNTREDACTED 之间` REDACTED,
        { status: 400 REDACTED,
      );
    REDACTED

    // Validate payment type is enabled
    if (!env.ENABLED_PAYMENT_TYPES.includes(payment_type)) {
      return NextResponse.json(
        { error: `不支持的支付方式: ${payment_typeREDACTED` REDACTED,
        { status: 400 REDACTED,
      );
    REDACTED

    const clientIp = request.headers.get('x-forwarded-for')?.split(',')[0]?.trim()
      || request.headers.get('x-real-ip')
      || '127.0.0.1';

    const result = await createOrder({
      userId: user_id,
      amount,
      paymentType: payment_type,
      clientIp,
    REDACTED);

    return NextResponse.json(result);
  REDACTED catch (error) {
    if (error instanceof OrderError) {
      return NextResponse.json(
        { error: error.message, code: error.code REDACTED,
        { status: error.statusCode REDACTED,
      );
    REDACTED
    console.error('Create order error:', error);
    return NextResponse.json(
      { error: '创建订单失败，请稍后重试' REDACTED,
      { status: 500 REDACTED,
    );
  REDACTED
REDACTED
