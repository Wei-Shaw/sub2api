import { NextRequest, NextResponse REDACTED from 'next/server';
import { verifyAdminToken, unauthorizedResponse REDACTED from '@/lib/admin-auth';
import { adminCancelOrder, OrderError REDACTED from '@/lib/order/service';

export async function POST(
  request: NextRequest,
  { params REDACTED: { params: Promise<{ id: string REDACTED> REDACTED,
) {
  if (!verifyAdminToken(request)) return unauthorizedResponse();

  try {
    const { id REDACTED = await params;
    await adminCancelOrder(id);
    return NextResponse.json({ success: true REDACTED);
  REDACTED catch (error) {
    if (error instanceof OrderError) {
      return NextResponse.json(
        { error: error.message, code: error.code REDACTED,
        { status: error.statusCode REDACTED,
      );
    REDACTED
    console.error('Admin cancel order error:', error);
    return NextResponse.json({ error: '取消订单失败' REDACTED, { status: 500 REDACTED);
  REDACTED
REDACTED
