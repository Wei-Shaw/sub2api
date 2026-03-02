import { NextRequest, NextResponse REDACTED from 'next/server';
import { prisma REDACTED from '@/lib/db';
import { verifyAdminToken, unauthorizedResponse REDACTED from '@/lib/admin-auth';
import { Prisma REDACTED from '@prisma/client';

export async function GET(request: NextRequest) {
  if (!verifyAdminToken(request)) return unauthorizedResponse();

  const searchParams = request.nextUrl.searchParams;
  const page = Math.max(1, Number(searchParams.get('page') || '1'));
  const pageSize = Math.min(100, Math.max(1, Number(searchParams.get('page_size') || '20')));
  const status = searchParams.get('status');
  const userId = searchParams.get('user_id');
  const dateFrom = searchParams.get('date_from');
  const dateTo = searchParams.get('date_to');

  const where: Prisma.OrderWhereInput = {REDACTED;
  if (status) where.status = status as any;
  if (userId) where.userId = Number(userId);
  if (dateFrom || dateTo) {
    where.createdAt = {REDACTED;
    if (dateFrom) where.createdAt.gte = new Date(dateFrom);
    if (dateTo) where.createdAt.lte = new Date(dateTo);
  REDACTED

  const [orders, total] = await Promise.all([
    prisma.order.findMany({
      where,
      orderBy: { createdAt: 'desc' REDACTED,
      skip: (page - 1) * pageSize,
      take: pageSize,
      select: {
        id: true,
        userId: true,
        userName: true,
        userEmail: true,
        amount: true,
        status: true,
        paymentType: true,
        createdAt: true,
        paidAt: true,
        completedAt: true,
        failedReason: true,
        expiresAt: true,
      REDACTED,
    REDACTED),
    prisma.order.count({ where REDACTED),
  ]);

  return NextResponse.json({
    orders: orders.map(o => ({
      ...o,
      amount: Number(o.amount),
    REDACTED)),
    total,
    page,
    page_size: pageSize,
    total_pages: Math.ceil(total / pageSize),
  REDACTED);
REDACTED
