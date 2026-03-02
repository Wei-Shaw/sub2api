import { NextRequest, NextResponse REDACTED from 'next/server';
import { prisma REDACTED from '@/lib/db';

export async function GET(
  request: NextRequest,
  { params REDACTED: { params: Promise<{ id: string REDACTED> REDACTED,
) {
  const { id REDACTED = await params;

  const order = await prisma.order.findUnique({
    where: { id REDACTED,
    select: {
      id: true,
      userId: true,
      userName: true,
      amount: true,
      status: true,
      paymentType: true,
      payUrl: true,
      qrCode: true,
      qrCodeImg: true,
      expiresAt: true,
      paidAt: true,
      completedAt: true,
      failedReason: true,
      createdAt: true,
    REDACTED,
  REDACTED);

  if (!order) {
    return NextResponse.json({ error: '订单不存在' REDACTED, { status: 404 REDACTED);
  REDACTED

  return NextResponse.json({
    order_id: order.id,
    user_id: order.userId,
    user_name: order.userName,
    amount: Number(order.amount),
    status: order.status,
    payment_type: order.paymentType,
    pay_url: order.payUrl,
    qr_code: order.qrCode,
    qr_code_img: order.qrCodeImg,
    expires_at: order.expiresAt,
    paid_at: order.paidAt,
    completed_at: order.completedAt,
    failed_reason: order.failedReason,
    created_at: order.createdAt,
  REDACTED);
REDACTED
