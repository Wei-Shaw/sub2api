import { NextRequest, NextResponse REDACTED from 'next/server';
import { prisma REDACTED from '@/lib/db';
import { getCurrentUserByToken REDACTED from '@/lib/sub2api/client';

export async function GET(request: NextRequest) {
  const token = request.nextUrl.searchParams.get('token')?.trim();
  if (!token) {
    return NextResponse.json({ error: 'token is required' REDACTED, { status: 400 REDACTED);
  REDACTED

  try {
    const user = await getCurrentUserByToken(token);
    const orders = await prisma.order.findMany({
      where: { userId: user.id REDACTED,
      orderBy: { createdAt: 'desc' REDACTED,
      take: 20,
      select: {
        id: true,
        amount: true,
        status: true,
        paymentType: true,
        createdAt: true,
      REDACTED,
    REDACTED);

    return NextResponse.json({
      user: {
        id: user.id,
        username: user.username,
        email: user.email,
        displayName: user.username || user.email || `用户 #${user.idREDACTED`,
        balance: user.balance,
      REDACTED,
      orders: orders.map((item) => ({
        id: item.id,
        amount: Number(item.amount),
        status: item.status,
        paymentType: item.paymentType,
        createdAt: item.createdAt,
      REDACTED)),
    REDACTED);
  REDACTED catch (error) {
    console.error('Get my orders error:', error);
    return NextResponse.json({ error: 'unauthorized' REDACTED, { status: 401 REDACTED);
  REDACTED
REDACTED
