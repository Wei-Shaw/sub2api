import { NextRequest, NextResponse REDACTED from 'next/server';
import { prisma REDACTED from '@/lib/db';
import { verifyAdminToken, unauthorizedResponse REDACTED from '@/lib/admin-auth';

export async function GET(
  request: NextRequest,
  { params REDACTED: { params: Promise<{ id: string REDACTED> REDACTED,
) {
  if (!verifyAdminToken(request)) return unauthorizedResponse();

  const { id REDACTED = await params;

  const order = await prisma.order.findUnique({
    where: { id REDACTED,
    include: {
      auditLogs: {
        orderBy: { createdAt: 'desc' REDACTED,
      REDACTED,
    REDACTED,
  REDACTED);

  if (!order) {
    return NextResponse.json({ error: '订单不存在' REDACTED, { status: 404 REDACTED);
  REDACTED

  return NextResponse.json({
    ...order,
    amount: Number(order.amount),
    refundAmount: order.refundAmount ? Number(order.refundAmount) : null,
  REDACTED);
REDACTED
