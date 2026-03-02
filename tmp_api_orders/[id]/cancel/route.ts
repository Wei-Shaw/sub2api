import { NextRequest, NextResponse REDACTED from 'next/server';
import { z REDACTED from 'zod';
import { cancelOrder, OrderError REDACTED from '@/lib/order/service';

const cancelSchema = z.object({
  user_id: z.number().int().positive(),
REDACTED);

export async function POST(
  request: NextRequest,
  { params REDACTED: { params: Promise<{ id: string REDACTED> REDACTED,
) {
  try {
    const { id REDACTED = await params;
    const body = await request.json();
    const parsed = cancelSchema.safeParse(body);

    if (!parsed.success) {
      return NextResponse.json(
        { error: '参数错误', details: parsed.error.flatten().fieldErrors REDACTED,
        { status: 400 REDACTED,
      );
    REDACTED

    await cancelOrder(id, parsed.data.user_id);
    return NextResponse.json({ success: true REDACTED);
  REDACTED catch (error) {
    if (error instanceof OrderError) {
      return NextResponse.json(
        { error: error.message, code: error.code REDACTED,
        { status: error.statusCode REDACTED,
      );
    REDACTED
    console.error('Cancel order error:', error);
    return NextResponse.json({ error: '取消订单失败' REDACTED, { status: 500 REDACTED);
  REDACTED
REDACTED
