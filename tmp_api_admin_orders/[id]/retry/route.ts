import { NextRequest, NextResponse REDACTED from 'next/server';
import { verifyAdminToken, unauthorizedResponse REDACTED from '@/lib/admin-auth';
import { retryRecharge, OrderError REDACTED from '@/lib/order/service';

export async function POST(
  request: NextRequest,
  { params REDACTED: { params: Promise<{ id: string REDACTED> REDACTED,
) {
  if (!verifyAdminToken(request)) return unauthorizedResponse();

  try {
    const { id REDACTED = await params;
    await retryRecharge(id);
    return NextResponse.json({ success: true REDACTED);
  REDACTED catch (error) {
    if (error instanceof OrderError) {
      return NextResponse.json(
        { error: error.message, code: error.code REDACTED,
        { status: error.statusCode REDACTED,
      );
    REDACTED
    console.error('Retry recharge error:', error);
    return NextResponse.json({ error: '重试充值失败' REDACTED, { status: 500 REDACTED);
  REDACTED
REDACTED
