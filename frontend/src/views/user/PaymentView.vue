<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl text-[#141413]">
      <div v-if="loading" class="flex items-center justify-center py-24">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-gray-900 border-t-transparent dark:border-white dark:border-t-transparent"></div>
      </div>

      <template v-else>
        <PaymentStatusPanel
          v-if="paymentPhase === 'paying'"
          :order-id="paymentState.orderId"
          :qr-code="paymentState.qrCode"
          :expires-at="paymentState.expiresAt"
          :payment-type="paymentState.paymentType"
          :pay-url="paymentState.payUrl"
          :order-type="paymentState.orderType"
          @done="onPaymentDone"
          @success="onPaymentSuccess"
          @settled="onPaymentSettled"
        />

        <template v-else-if="selectedPlan">
          <div class="mb-4">
            <button class="inline-flex items-center gap-2 text-sm font-medium text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white" @click="selectedPlan = null">
              <span>&larr;</span><span>返回套餐选择</span>
            </button>
          </div>
          <div class="grid gap-5 lg:grid-cols-5">
            <section class="overflow-hidden rounded-[28px] border border-gray-900 bg-[#fffdf7] dark:border-gray-600 dark:bg-gray-900 lg:col-span-3">
              <div class="border-b border-gray-900 bg-[#f6efe1] px-6 py-4 dark:border-gray-700 dark:bg-gray-800">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="rounded-full border border-gray-900 bg-gray-900 px-3 py-1 text-xs font-semibold text-white dark:border-white dark:bg-white dark:text-gray-900">{{ selectedPlan.name }}</span>
                  <span class="rounded-full border border-gray-300 px-3 py-1 text-xs text-gray-500 dark:border-gray-600 dark:text-gray-400">{{ planBadge(selectedPlan) }}</span>
                </div>
              </div>
              <div class="p-6 sm:p-8">
                <p class="text-sm font-semibold uppercase tracking-[0.3em] text-gray-400">确认这一档</p>
                <div class="mt-4 flex flex-wrap items-end gap-3">
                  <span v-if="selectedPlan.original_price" class="mb-2 text-lg text-gray-400 line-through">&yen;{{ selectedPlan.original_price }}</span>
                  <span class="text-5xl font-black tracking-tight text-gray-950 dark:text-white">&yen;{{ planDisplayAmount(selectedPlan) }}</span>
                  <span class="mb-2 text-sm text-gray-500">{{ planPriceSuffix(selectedPlan) }}</span>
                </div>
                <p class="mt-4 max-w-xl text-2xl font-bold leading-tight text-gray-950 dark:text-white">{{ planTagline(selectedPlan) }}</p>
                <p class="mt-3 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ planValueLine(selectedPlan) }}</p>
                <div class="mt-6 grid gap-3 sm:grid-cols-3">
                  <div v-for="item in planBullets(selectedPlan)" :key="item" class="rounded-2xl border border-gray-200 bg-white px-4 py-3 text-sm font-medium text-gray-700 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-200">
                    {{ item }}
                  </div>
                </div>
              </div>
            </section>

            <aside class="rounded-[28px] border border-gray-900 bg-white p-6 dark:border-gray-600 dark:bg-gray-800 lg:col-span-2">
              <p class="text-sm font-semibold text-gray-900 dark:text-white">用充值余额买套餐</p>
              <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-gray-400">套餐只扣充值余额。赠送余额留着按量调用，等于多一层缓冲。</p>
              <div class="mt-5 grid gap-3">
                <div class="rounded-2xl border border-gray-200 bg-[#fffaf0] px-4 py-3 dark:border-gray-700 dark:bg-gray-900/60">
                  <p class="text-xs text-gray-500">可购买套餐</p>
                  <p class="mt-1 text-2xl font-black text-gray-950 dark:text-white">${{ cashBalanceDisplay }}</p>
                  <p class="mt-1 text-xs text-gray-500">充值余额</p>
                </div>
                <div class="rounded-2xl border border-dashed border-gray-300 px-4 py-3 dark:border-gray-600">
                  <p class="text-xs text-gray-500">赠送余额</p>
                  <p class="mt-1 text-lg font-bold text-gray-900 dark:text-white">${{ giftBalanceDisplay }}</p>
                  <p class="mt-1 text-xs text-gray-500">不买套餐，只抵扣按量调用</p>
                </div>
              </div>
              <button class="mt-5 w-full rounded-2xl bg-gray-950 py-3.5 text-sm font-semibold text-white transition-colors hover:bg-gray-800 disabled:opacity-50 dark:bg-white dark:text-gray-950" :disabled="submitting" @click="purchaseSelectedPlanWithBalance">
                <span v-if="submitting">购买中...</span><span v-else>{{ planButtonText(selectedPlan) }}</span>
              </button>
              <button class="mt-3 w-full rounded-2xl border border-gray-900 py-3 text-sm font-semibold text-gray-900 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-500 dark:text-white dark:hover:bg-gray-700" :disabled="submitting || !visibleMethods.alipay" @click="purchasePlanWithAlipay(selectedPlan)">
                支付宝支付
              </button>
              <router-link to="/redeem" class="mt-3 block text-center text-xs font-medium text-gray-500 underline underline-offset-4 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white">已有兑换码，去兑换</router-link>
            </aside>
          </div>
        </template>

        <template v-else-if="checkoutMode">
          <div class="mx-auto max-w-xl space-y-5">
            <button class="text-sm font-medium text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white" @click="closeCheckout">&larr; 返回选择</button>
            <div class="rounded-[28px] border border-gray-900 bg-white p-6 dark:border-gray-600 dark:bg-gray-800">
              <div class="mb-5 flex items-center justify-between">
                <div>
                  <p class="text-xs uppercase tracking-[0.28em] text-gray-400">checkout</p>
                  <h2 class="mt-1 text-2xl font-black text-gray-950 dark:text-white">{{ checkoutTitle }}</h2>
                </div>
                <span class="rounded-full border border-[#1677FF] px-3 py-1 text-xs font-semibold text-[#1677FF]">支付宝</span>
              </div>

              <div class="rounded-2xl border border-gray-200 bg-[#fffdf7] p-4 dark:border-gray-700 dark:bg-gray-900/60">
                <p class="text-sm font-semibold text-gray-900 dark:text-white">付款前再看一眼</p>
                <div class="mt-4 space-y-3 text-sm">
                  <div v-if="checkoutMode === 'subscription' && checkoutPlan" class="flex justify-between gap-4">
                    <span class="text-gray-500">套餐</span><span class="font-semibold text-gray-900 dark:text-white">{{ checkoutPlan.name }}</span>
                  </div>
                  <div class="flex justify-between gap-4">
                    <span class="text-gray-500">金额</span><span class="font-semibold text-gray-900 dark:text-white">¥{{ checkoutBaseAmount.toFixed(2) }}</span>
                  </div>
                  <div class="flex justify-between gap-4">
                    <span class="text-gray-500">手续费</span><span class="font-semibold text-gray-900 dark:text-white">¥{{ checkoutFeeAmount.toFixed(2) }}</span>
                  </div>
                  <div class="border-t border-gray-200 pt-3 dark:border-gray-700">
                    <div class="flex items-center justify-between gap-4">
                      <span class="font-medium text-gray-700 dark:text-gray-200">实付</span>
                      <span class="text-3xl font-black text-gray-950 dark:text-white">¥{{ checkoutPayAmount.toFixed(2) }}</span>
                    </div>
                  </div>
                  <div class="flex justify-between gap-4">
                    <span class="text-gray-500">{{ checkoutMode === 'subscription' ? '等值扣除 API 余额' : '到账 API 余额' }}</span>
                    <span class="font-semibold text-gray-900 dark:text-white">${{ checkoutReceiveBalance.toFixed(2) }}</span>
                  </div>
                </div>
                <p class="mt-3 text-xs text-gray-400">1 元 = {{ safeMultiplier }} API 余额。套餐买完默认生效。</p>
              </div>

              <button class="mt-5 w-full rounded-2xl bg-gray-950 py-3.5 text-sm font-semibold text-white transition-colors hover:bg-gray-800 disabled:opacity-50 dark:bg-white dark:text-gray-950" :disabled="submitting || checkoutBaseAmount <= 0" @click="confirmAlipayCheckout">
                <span v-if="submitting">生成支付宝订单中...</span><span v-else>{{ checkoutButtonText }}</span>
              </button>
            </div>
          </div>
        </template>

        <template v-else>
          <section class="overflow-hidden rounded-[32px] border border-[#e8e6dc] bg-[#f5f4ed] shadow-[rgba(50,50,93,0.16)_0px_30px_45px_-30px,rgba(0,0,0,0.08)_0px_18px_36px_-18px] dark:border-gray-600 dark:bg-gray-900">
            <div class="grid gap-0 lg:grid-cols-[1.05fr_0.95fr]">
              <div class="p-6 sm:p-8 lg:p-10">
                <div class="inline-flex rounded-full border border-[#d1cfc5] bg-[#faf9f5] px-3 py-1 text-xs font-semibold text-[#4d4c48] dark:border-gray-500 dark:bg-gray-800 dark:text-white">不用先买一个月 · 用得多再开套餐</div>
                <h1 class="mt-6 max-w-2xl text-4xl font-semibold leading-[1.02] tracking-[-0.04em] text-[#141413] dark:text-white sm:text-6xl">
                  今天要跑 Agent，<br />就今天开。
                </h1>
                <p class="mt-5 max-w-xl text-base leading-7 text-[#5e5d59] dark:text-gray-300">日卡冲刺、周卡最稳、月卡日均最低。你买的是一段可控的高强度 AI 编程时间，不是被迫订一个月。</p>
                <div class="mt-7 grid gap-3 sm:grid-cols-3">
                  <div class="rounded-2xl border border-[#e8e6dc] bg-[#faf9f5] px-4 py-3 shadow-[0_0_0_1px_#f0eee6] dark:border-gray-700 dark:bg-gray-800">
                    <p class="text-xs text-gray-500">新用户奖励</p>
                    <p class="mt-1 text-2xl font-black text-gray-950 dark:text-white">$10</p>
                    <p class="mt-1 text-xs text-gray-500">先按量试用</p>
                  </div>
                  <div class="rounded-2xl border border-[#e8e6dc] bg-[#faf9f5] px-4 py-3 shadow-[0_0_0_1px_#f0eee6] dark:border-gray-700 dark:bg-gray-800">
                    <p class="text-xs text-gray-500">充值换算</p>
                    <p class="mt-1 text-2xl font-black text-gray-950 dark:text-white">1={{ safeMultiplier }}</p>
                    <p class="mt-1 text-xs text-gray-500">1 元 = {{ safeMultiplier }} API 余额</p>
                  </div>
                  <div class="rounded-2xl border border-[#0e0f0c] bg-[#0e0f0c] px-4 py-3 text-white shadow-[rgba(50,50,93,0.22)_0px_18px_36px_-18px] dark:border-gray-700 dark:bg-white dark:text-gray-950">
                    <p class="text-xs opacity-70">最推荐</p>
                    <p class="mt-1 text-2xl font-black">周卡</p>
                    <p class="mt-1 text-xs opacity-70">一周开发最划算</p>
                  </div>
                </div>
              </div>

              <div class="border-t border-[#141413] bg-[#141413] p-5 dark:border-gray-700 lg:border-l lg:border-t-0">
                <div class="h-full rounded-[24px] border border-white/20 bg-black p-5 font-mono text-sm text-white">
                  <div class="mb-5 flex items-center gap-2 border-b border-white/15 pb-4">
                    <span class="h-3 w-3 rounded-full bg-[#ff5f57]"></span>
                    <span class="h-3 w-3 rounded-full bg-[#ffbd2e]"></span>
                    <span class="h-3 w-3 rounded-full bg-[#28c840]"></span>
                    <span class="ml-auto text-xs text-white/40">mcorgai.run</span>
                  </div>
                  <div class="space-y-4 text-white/80">
                    <p><span class="text-[#79ffe1]">$</span> 今天赶项目</p>
                    <p class="pl-4 text-white">→ 开日卡，马上跑</p>
                    <p><span class="text-[#79ffe1]">$</span> 一周持续开发</p>
                    <p class="pl-4 text-white">→ 周卡，日均更低</p>
                    <p><span class="text-[#79ffe1]">$</span> 长期高频使用</p>
                    <p class="pl-4 text-white">→ 月卡，最省心</p>
                  </div>
                  <div class="mt-8 rounded-2xl border border-[#9fe870]/50 bg-[#9fe870]/10 p-4 text-xs leading-6 text-[#eaffdf]">
                    不想订阅？也可以只按量扣余额。赠送余额优先抵扣按量调用，充值余额再兜底。
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section class="mt-6 grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
            <div class="rounded-[28px] border border-[#e5edf5] bg-white p-5 shadow-[rgba(50,50,93,0.12)_0px_20px_35px_-25px,rgba(0,0,0,0.08)_0px_10px_24px_-18px] dark:border-gray-600 dark:bg-gray-800">
              <div class="flex flex-wrap items-center justify-between gap-4">
                <div>
                  <p class="text-xs uppercase tracking-[0.28em] text-gray-400">wallet</p>
                  <h2 class="mt-1 text-xl font-black text-gray-950 dark:text-white">余额拆开看，心里更清楚</h2>
                </div>
                <p class="text-sm text-gray-500">总可用 ${{ totalBalanceDisplay }}</p>
              </div>
              <div class="mt-4 grid gap-3 sm:grid-cols-3">
                <div class="rounded-2xl border border-gray-200 bg-[#fffaf0] px-4 py-3 dark:border-gray-700 dark:bg-gray-900/60">
                  <p class="text-xs text-gray-500">充值余额</p>
                  <p class="mt-1 text-2xl font-black text-gray-950 dark:text-white">${{ cashBalanceDisplay }}</p>
                  <p class="mt-1 text-xs text-gray-500">买套餐 / 按量都能用</p>
                </div>
                <div class="rounded-2xl border border-gray-200 px-4 py-3 dark:border-gray-700">
                  <p class="text-xs text-gray-500">赠送余额</p>
                  <p class="mt-1 text-2xl font-black text-gray-950 dark:text-white">${{ giftBalanceDisplay }}</p>
                  <p class="mt-1 text-xs text-gray-500">只抵扣按量调用</p>
                </div>
                <div class="rounded-2xl border border-gray-200 px-4 py-3 dark:border-gray-700">
                  <p class="text-xs text-gray-500">冻结赠送</p>
                  <p class="mt-1 text-2xl font-black text-gray-950 dark:text-white">${{ frozenGiftBalanceDisplay }}</p>
                  <p class="mt-1 text-xs text-gray-500">24 小时后解冻</p>
                </div>
              </div>
            </div>
            <div class="rounded-[28px] border border-[#d1cfc5] bg-[#faf9f5] p-5 shadow-[0_0_0_1px_#f0eee6] dark:border-gray-600 dark:bg-gray-900">
              <p class="text-xs uppercase tracking-[0.28em] text-gray-500">how to pick</p>
              <h2 class="mt-1 text-xl font-black text-gray-950 dark:text-white">怎么选</h2>
              <div class="mt-4 space-y-2 text-sm font-medium text-gray-700 dark:text-gray-300">
                <p>偶尔试试：先按量。</p>
                <p>今天赶项目：日卡。</p>
                <p>一周都在开发：周卡。</p>
                <p>每天离不开：月卡。</p>
              </div>
            </div>
          </section>

          <section class="mt-8">
            <div class="mb-4 flex flex-wrap items-end justify-between gap-3">
              <div>
                <p class="text-xs uppercase tracking-[0.3em] text-gray-400">plans</p>
                <h2 class="mt-1 text-3xl font-black tracking-tight text-gray-950 dark:text-white">选择你的 AI 编程时间</h2>
              </div>
              <p class="text-sm text-gray-500 dark:text-gray-400">价格越长，日均越低；买完默认生效。</p>
            </div>

            <div v-if="checkout.plans.length === 0" class="rounded-[28px] border border-gray-200 bg-white py-16 text-center dark:border-gray-700 dark:bg-gray-800">
              <p class="text-gray-400">暂无套餐</p>
            </div>
            <div v-else class="grid gap-4 lg:grid-cols-3">
              <article
                v-for="plan in sortedPlans"
                :key="plan.id"
                class="relative flex min-h-[430px] flex-col rounded-[28px] border bg-white p-5 shadow-[rgba(50,50,93,0.10)_0px_18px_34px_-26px,rgba(0,0,0,0.06)_0px_10px_22px_-18px] transition-transform hover:-translate-y-1 dark:bg-gray-800"
                :class="plan._recommended ? 'border-[#0e0f0c] bg-[#f5f4ed] dark:border-white dark:bg-gray-900' : 'border-[#e5edf5] dark:border-gray-700'"
              >
                <div v-if="plan._recommended" class="absolute -top-3 left-5 rounded-full border border-[#163300] bg-[#9fe870] px-3 py-1 text-xs font-bold text-[#163300] shadow-[0_0_0_1px_rgba(14,15,12,0.12)] dark:border-white dark:bg-white dark:text-gray-950">多数人选这个</div>
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <p class="text-sm font-bold text-gray-500">{{ planBadge(plan) }}</p>
                    <h3 class="mt-1 text-3xl font-black tracking-tight text-gray-950 dark:text-white">{{ plan.name }}</h3>
                  </div>
                  <button class="rounded-full border border-gray-200 px-3 py-1 text-xs font-semibold text-gray-500 hover:border-gray-900 hover:text-gray-900 dark:border-gray-700 dark:hover:border-white dark:hover:text-white" @click="selectedPlan = plan">详情</button>
                </div>

                <div class="mt-6">
                  <div class="flex items-end gap-2">
                    <span v-if="plan.original_price" class="mb-1 text-sm text-gray-400 line-through">&yen;{{ plan.original_price }}</span>
                    <span class="text-5xl font-black tracking-tight text-gray-950 dark:text-white">&yen;{{ planDisplayAmount(plan) }}</span>
                    <span class="mb-2 text-sm text-gray-500">{{ planPriceSuffix(plan) }}</span>
                  </div>
                  <p class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">{{ planValueLine(plan) }}</p>
                  <p class="mt-1 text-xs text-gray-400">余额支付约扣 ${{ planDeductBalance(plan) }} 充值余额</p>
                </div>

                <div class="mt-6 rounded-2xl border border-gray-200 bg-white/70 p-4 dark:border-gray-700 dark:bg-gray-900/50">
                  <p class="text-lg font-black leading-6 text-gray-950 dark:text-white">{{ planTagline(plan) }}</p>
                  <div class="mt-4 space-y-2 text-sm text-gray-600 dark:text-gray-300">
                    <p v-for="item in planBullets(plan)" :key="item" class="border-l border-gray-900 pl-3 dark:border-white">{{ item }}</p>
                  </div>
                </div>

                <div class="mt-auto pt-5">
                  <p v-if="plan.purchase_quote?.action === 'extend'" class="mb-3 text-xs font-semibold text-green-700 dark:text-green-400">当前套餐，再买自动延期</p>
                  <p v-else class="mb-3 text-xs font-semibold text-green-700 dark:text-green-400">购买后默认直接生效</p>
                  <div class="grid gap-2">
                    <button class="rounded-full bg-[#0e0f0c] py-3 text-sm font-semibold text-white transition-transform hover:scale-[1.02] disabled:opacity-50 dark:bg-white dark:text-gray-950" :disabled="submitting || !visibleMethods.alipay" @click="purchasePlanWithAlipay(plan)">
                      <span v-if="submitting && checkoutPlan?.id === plan.id">生成二维码中...</span><span v-else>支付宝支付</span>
                    </button>
                    <button class="rounded-full border border-[#0e0f0c] py-3 text-sm font-semibold text-[#0e0f0c] transition-transform hover:scale-[1.02] hover:bg-[#f5f4ed] disabled:opacity-50 dark:border-gray-500 dark:text-white dark:hover:bg-gray-700" :disabled="submitting" @click="purchasePlanWithBalance(plan)">
                      <span v-if="submitting && selectedPlan?.id === plan.id">购买中...</span><span v-else>{{ planButtonText(plan) }}</span>
                    </button>
                  </div>
                </div>
              </article>
            </div>
          </section>

          <section class="mt-8 grid gap-4 lg:grid-cols-2">
            <div class="rounded-[28px] border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
              <p class="text-xs uppercase tracking-[0.28em] text-gray-400">pay as you go</p>
              <h2 class="mt-1 text-xl font-black text-gray-950 dark:text-white">不买套餐，也能直接用</h2>
              <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-gray-400">充值后选择按量分组，按每次 API 调用扣费。赠送余额会优先抵扣，适合先试、偶尔用、临时补一点。</p>
              <div class="mt-4 flex flex-wrap items-center gap-2">
                <button v-for="amt in [10, 20, 50, 100, 200, 500]" :key="amt" class="rounded-xl border px-3 py-2 text-sm font-semibold transition-all active:scale-[0.97]" :class="qqSelectedAmount === amt ? 'border-gray-950 bg-gray-950 text-white dark:border-white dark:bg-white dark:text-gray-950' : 'border-gray-200 text-gray-600 hover:border-gray-900 dark:border-gray-700 dark:text-gray-300'" @click="customAmount = ''; qqSelectedAmount = amt">&yen;{{ amt }}</button>
              </div>
              <div class="relative mt-3">
                <span class="pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm text-gray-400">&yen;</span>
                <input v-model="customAmount" type="number" min="1" step="0.01" placeholder="任意金额" class="w-full rounded-2xl border border-gray-200 bg-white py-3 pl-7 pr-16 text-sm text-gray-900 placeholder-gray-400 outline-none transition-colors focus:border-gray-900 dark:border-gray-700 dark:bg-gray-900 dark:text-white dark:focus:border-white" @input="qqSelectedAmount = 0" />
                <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-gray-400">人民币</span>
              </div>
              <p v-if="estimatedBalance > 0" class="mt-3 text-sm text-gray-500 dark:text-gray-400">预计到账 <span class="font-black text-gray-950 dark:text-white">${{ estimatedBalance.toFixed(2) }}</span> API 余额</p>
              <button class="mt-4 w-full rounded-2xl bg-gray-950 py-3 text-sm font-semibold text-white transition-colors hover:bg-gray-800 disabled:opacity-50 dark:bg-white dark:text-gray-950" :disabled="submitting || !canPayRechargeWithAlipay" @click="rechargeWithAlipay">
                <span v-if="submitting">生成二维码中...</span><span v-else>支付宝扫码充值</span>
              </button>
              <p v-if="!visibleMethods.alipay" class="mt-2 text-xs text-red-500">当前未启用支付宝支付，请稍后再试。</p>
            </div>

            <div class="space-y-4">
              <div class="rounded-[28px] border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
                <p class="text-sm font-bold text-gray-900 dark:text-white">兑换码 / 优惠码</p>
                <div class="mt-4 space-y-3">
                  <div>
                    <p class="mb-2 text-xs text-gray-500">兑换码充值：到账充值余额，可买套餐</p>
                    <div class="flex gap-2">
                      <input v-model="redeemCode" type="text" placeholder="输入兑换码" class="min-w-0 flex-1 rounded-2xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-900 outline-none focus:border-gray-900 dark:border-gray-700 dark:bg-gray-900 dark:text-white dark:focus:border-white" @keyup.enter="handleRedeem" />
                      <button class="rounded-2xl bg-gray-950 px-5 py-3 text-sm font-semibold text-white disabled:opacity-50 dark:bg-white dark:text-gray-950" :disabled="!redeemCode || redeeming" @click="handleRedeem"><span v-if="redeeming">兑换中</span><span v-else>兑换</span></button>
                    </div>
                    <p v-if="redeemError" class="mt-2 text-xs text-red-500">{{ redeemError }}</p>
                    <p v-if="redeemSuccess" class="mt-2 text-xs text-green-600">{{ redeemSuccess }}</p>
                  </div>
                  <div>
                    <p class="mb-2 text-xs text-gray-500">优惠码领取：赠送余额，只能按量抵扣</p>
                    <div class="flex gap-2">
                      <input v-model="promoCode" type="text" placeholder="输入优惠码" class="min-w-0 flex-1 rounded-2xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-900 outline-none focus:border-gray-900 dark:border-gray-700 dark:bg-gray-900 dark:text-white dark:focus:border-white" @keyup.enter="handleRedeemPromo" />
                      <button class="rounded-2xl border border-gray-900 px-5 py-3 text-sm font-semibold text-gray-900 disabled:opacity-50 dark:border-gray-500 dark:text-white" :disabled="!promoCode || promoRedeeming" @click="handleRedeemPromo"><span v-if="promoRedeeming">领取中</span><span v-else>领取</span></button>
                    </div>
                    <p v-if="promoError" class="mt-2 text-xs text-red-500">{{ promoError }}</p>
                    <p v-if="promoSuccess" class="mt-2 text-xs text-green-600">{{ promoSuccess }}</p>
                  </div>
                </div>
              </div>

              <div class="rounded-[28px] border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
                <p class="text-sm font-bold text-gray-900 dark:text-white">用户交流群</p>
                <p class="mt-2 text-2xl font-black text-gray-950 dark:text-white">774692252</p>
                <p class="mt-1 text-xs text-gray-500">活动福利 · 更新通知 · 使用答疑</p>
              </div>
            </div>
          </section>

          <section v-if="activeSubscriptions.length > 0" class="mt-8 rounded-[28px] border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
            <h3 class="text-sm font-bold text-gray-900 dark:text-white">当前订阅</h3>
            <div class="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <div v-for="sub in activeSubscriptions" :key="sub.id" class="flex items-center justify-between gap-3 rounded-2xl border border-gray-200 px-4 py-3 dark:border-gray-700">
                <div class="min-w-0">
                  <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ sub.group?.name || 'Group ' + sub.group_id }}</p>
                  <p class="mt-0.5 text-xs text-gray-400"><span v-if="sub.expires_at">剩余 {{ getDaysRemaining(sub.expires_at) }} 天</span><span v-else>永久有效</span></p>
                </div>
                <span class="rounded-full border border-green-600 px-2 py-0.5 text-[10px] font-bold text-green-700 dark:border-green-400 dark:text-green-400">生效中</span>
              </div>
            </div>
          </section>

          <details class="mt-8 rounded-[28px] border border-gray-200 bg-white p-5 text-sm text-gray-500 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-400">
            <summary class="cursor-pointer text-sm font-bold text-gray-900 dark:text-white">使用规则和风控说明</summary>
            <div class="mt-4 space-y-2 leading-6">
              <p>账号仅限本人使用，禁止共享、转售或代挂；违规将永久封禁且不退款。</p>
              <p>按量计费优先扣赠送余额，不足再扣充值余额；套餐购买只扣充值余额。</p>
              <p>注册和邀请奖励属于赠送余额，24 小时冻结规则用于防刷；完整条款见 <a href="/terms" class="underline underline-offset-4">使用条款</a>。</p>
            </div>
          </details>
        </template>
      </template>
    </div>

    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 backdrop-blur-sm" @click="previewImage = ''">
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-xl object-contain shadow-2xl" />
        </div>
      </Transition>
    </Teleport>

    <ConfirmDialog
      :show="showBalancePurchaseConfirm"
      title="确认购买套餐"
      :message="balancePurchaseConfirmMessage"
      :confirm-text="pendingBalancePlan ? planButtonText(pendingBalancePlan) : '确认购买'"
      cancel-text="再想想"
      @confirm="confirmBalancePurchase"
      @cancel="cancelBalancePurchase"
    />
  </AppLayout>
</template>

<script setup lang="ts">
// @ts-nocheck
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { redeemAPI } from '@/api'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import { METHOD_ORDER, getPaymentPopupFeatures } from '@/components/payment/providerConfig'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  buildCreateOrderPayload,
  clearPaymentRecoverySnapshot,
  decidePaymentLaunch,
  getVisibleMethods,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
  writePaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { platformLabel } from '@/utils/platformColors'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const subscriptionStore = useSubscriptionStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)
const formatBalance = (value?: number | null) => Number(value || 0).toFixed(2)
const totalBalanceDisplay = computed(() => formatBalance(user.value?.balance))
const cashBalanceDisplay = computed(() => formatBalance(user.value?.cash_balance ?? user.value?.balance))
const giftBalanceDisplay = computed(() => formatBalance(user.value?.gift_balance))
const frozenGiftBalanceDisplay = computed(() => formatBalance(user.value?.frozen_gift_balance))

function getDaysRemaining(expiresAt: string): number {
  const expires = new Date(expiresAt)
  if (Number.isNaN(expires.getTime())) return 0
  const now = new Date()
  const expiresDay = Date.UTC(expires.getFullYear(), expires.getMonth(), expires.getDate())
  const today = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())
  return Math.max(0, Math.round((expiresDay - today) / (1000 * 60 * 60 * 24)))
}

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('recharge')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const checkoutMode = ref<'recharge' | 'subscription' | null>(null)
const checkoutPlan = ref<SubscriptionPlan | null>(null)
const qqSelectedAmount = ref<number | null>(10)
const customAmount = ref('')
const safeMultiplier = computed(() => {
  const value = Number(checkout.value.balance_recharge_multiplier || 0)
  return value > 1 ? value : 10
})
const estimatedBalance = computed(() => {
  const amt = customAmount.value ? parseFloat(customAmount.value) : (qqSelectedAmount.value || 0)
  return amt > 0 ? amt * safeMultiplier.value : 0
})
const rechargeAmount = computed(() => {
  const amt = customAmount.value ? parseFloat(customAmount.value) : (qqSelectedAmount.value || 0)
  return Number.isFinite(amt) ? amt : 0
})
const canPayRechargeWithAlipay = computed(() => {
  const amt = rechargeAmount.value
  return !!visibleMethods.value.alipay && amt > 0 && amountFitsMethod(amt, 'alipay')
})
const checkoutBaseAmount = computed(() => checkoutMode.value === 'subscription' && checkoutPlan.value ? effectivePlanAmount(checkoutPlan.value) : rechargeAmount.value)
const checkoutFeeRate = computed(() => checkout.value.recharge_fee_rate || 0)
const checkoutFeeAmount = computed(() => checkoutBaseAmount.value * checkoutFeeRate.value / 100)
const checkoutPayAmount = computed(() => checkoutBaseAmount.value + checkoutFeeAmount.value)
const checkoutReceiveBalance = computed(() => checkoutBaseAmount.value * safeMultiplier.value)
const checkoutTitle = computed(() => checkoutMode.value === 'subscription' ? '套餐支付宝支付' : '支付宝扫码充值')
const checkoutButtonText = computed(() => `确认支付 ¥${checkoutPayAmount.value.toFixed(2)}`)
const redeemCode = ref('')
const redeeming = ref(false)
const redeemError = ref('')
const redeemSuccess = ref('')
const promoCode = ref('')
const promoRedeeming = ref(false)
const promoError = ref('')
const promoSuccess = ref('')
const showBalancePurchaseConfirm = ref(false)
const pendingBalancePlan = ref<SubscriptionPlan | null>(null)
const balancePurchaseConfirmMessage = computed(() => {
  const plan = pendingBalancePlan.value
  if (!plan) return ''
  const action = plan.purchase_quote?.action
  const cnyAmount = effectivePlanAmount(plan)
  const multiplier = safeMultiplier.value
  const usdAmount = (cnyAmount * multiplier).toFixed(2)
  const balance = cashBalanceDisplay.value
  if (action === 'extend') {
    return `确认续费「${plan.name}」吗？本次将从充值余额扣除 $${usdAmount}（¥${planDisplayAmount(plan)} × ${multiplier} 倍率），赠送余额不可用于购买套餐。购买成功后自动延长 ${plan.validity_days} 天。当前充值余额 $${balance}。`
  }
  return `确认购买「${plan.name}」吗？本次将从充值余额扣除 $${usdAmount}（¥${planDisplayAmount(plan)} × ${multiplier} 倍率），赠送余额不可用于购买套餐。购买成功后立即生效。当前充值余额 $${balance}。`
})

async function handleRedeem() {
  if (!redeemCode.value || redeeming.value) return
  redeeming.value = true
  redeemError.value = ''
  redeemSuccess.value = ''
  try {
    await redeemAPI.redeem(redeemCode.value.trim())
    redeemSuccess.value = '兑换成功！余额已更新'
    redeemCode.value = ''
    await authStore.refreshUser()
    setTimeout(() => { redeemSuccess.value = '' }, 3000)
  } catch (e: any) {
    redeemError.value = extractApiErrorMessage(e) || '兑换失败'
  } finally {
    redeeming.value = false
  }
}

async function handleRedeemPromo() {
  if (!promoCode.value || promoRedeeming.value) return
  promoRedeeming.value = true
  promoError.value = ''
  promoSuccess.value = ''
  try {
    const result = await redeemAPI.redeemPromo(promoCode.value.trim())
    promoSuccess.value = result?.bonus_amount
      ? `领取成功！已到账 $${result.bonus_amount.toFixed(2)}`
      : (result?.message || '领取成功！余额已更新')
    promoCode.value = ''
    await authStore.refreshUser()
    setTimeout(() => { promoSuccess.value = '' }, 3000)
  } catch (e: any) {
    promoError.value = extractApiErrorMessage(e) || '领取失败'
  } finally {
    promoRedeeming.value = false
  }
}
const previewImage = ref('')

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
}

interface WeixinJSBridgeLike {
  invoke(
    action: string,
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

function emptyPaymentState(): PaymentRecoverySnapshot {
  return {
    orderId: 0,
    amount: 0,
    qrCode: '',
    expiresAt: '',
    paymentType: '',
    payUrl: '',
    outTradeNo: '',
    clientSecret: '',
    payAmount: 0,
    orderType: '',
    paymentMode: '',
    resumeToken: '',
    createdAt: 0,
  }
}

function getWeixinJSBridge(): WeixinJSBridgeLike | undefined {
  return (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
}

function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike | null> {
  const existing = getWeixinJSBridge()
  if (existing) return Promise.resolve(existing)

  return new Promise((resolve) => {
    let settled = false
    const finish = (bridge: WeixinJSBridgeLike | null) => {
      if (settled) return
      settled = true
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
      resolve(bridge)
    }
    const handleReady = () => finish(getWeixinJSBridge() ?? null)
    const timer = window.setTimeout(() => finish(getWeixinJSBridge() ?? null), timeoutMs)
    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

async function invokeWechatJsapiPayment(payload: Record<string, unknown>): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge()
  if (!bridge) {
    throw new Error('WECHAT_JSAPI_UNAVAILABLE')
  }
  return new Promise((resolve) => {
    bridge.invoke('getBrandWCPayRequest', payload, (result) => resolve(result || {}))
  })
}

const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())

function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
  if (typeof window === 'undefined' || !snapshot.orderId) return
  writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
}

function removeRecoverySnapshot() {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function resetPayment() {
  paymentPhase.value = 'select'
  paymentState.value = emptyPaymentState()
  closeCheckout()
  removeRecoverySnapshot()
}

async function redirectToPaymentResult(state: PaymentRecoverySnapshot): Promise<void> {
  const query: Record<string, string | undefined> = {}
  if (state.orderId > 0) {
    query.order_id = String(state.orderId)
  }
  if (state.outTradeNo) {
    query.out_trade_no = state.outTradeNo
  }
  if (state.resumeToken) {
    query.resume_token = state.resumeToken
  }
  await router.push({
    path: '/payment/result',
    query,
  })
}

function buildWechatOAuthAuthorizeUrl(
  authorizeUrl: string,
  context: { paymentType: string; orderType: OrderType; planId?: number; orderAmount: number },
): string {
  const normalizedUrl = authorizeUrl.trim()
  if (!normalizedUrl || typeof window === 'undefined') {
    return normalizedUrl
  }

  try {
    const targetUrl = new URL(normalizedUrl, window.location.origin)
    const redirectPath = targetUrl.searchParams.get('redirect') || '/purchase'
    const redirectUrl = new URL(redirectPath, window.location.origin)
    const paymentType = normalizeVisibleMethod(context.paymentType) || context.paymentType.trim() || 'wxpay'

    redirectUrl.searchParams.set('payment_type', paymentType)
    redirectUrl.searchParams.set('order_type', context.orderType)

    if (context.planId) {
      redirectUrl.searchParams.set('plan_id', String(context.planId))
    } else {
      redirectUrl.searchParams.delete('plan_id')
    }

    if (context.orderAmount > 0) {
      redirectUrl.searchParams.set('amount', String(context.orderAmount))
    } else {
      redirectUrl.searchParams.delete('amount')
    }

    targetUrl.searchParams.set('redirect', `${redirectUrl.pathname}${redirectUrl.search}`)
    return targetUrl.toString()
  } catch {
    return normalizedUrl
  }
}

function onPaymentDone() {
  const wasSubscription = paymentState.value.orderType === 'subscription'
  resetPayment()
  selectedPlan.value = null
  if (wasSubscription) {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSuccess() {
  removeRecoverySnapshot()
  authStore.refreshUser()
  if (paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSettled() {
  removeRecoverySnapshot()
}

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, recharge_fee_rate: 0, help_text: '', help_image_url: '', stripe_publishable_key: '',
})

const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const sortedPlans = computed(() => {
  const plans = [...checkout.value.plans].sort((a, b) => a.price - b.price)
  return plans.map(p => ({ ...p, _recommended: p.name === '周卡' }))
})

function planFeatures(plan: SubscriptionPlan | null): string[] {
  if (!plan || !plan.features) return []
  if (Array.isArray(plan.features)) return plan.features.filter(Boolean)
  return String(plan.features).split('\n').map(item => item.trim()).filter(Boolean)
}

function effectivePlanAmount(plan: SubscriptionPlan): number {
  return Number(plan.purchase_quote?.display_amount ?? plan.purchase_quote?.amount ?? plan.price)
}

function planDisplayAmount(plan: SubscriptionPlan): string {
  const amount = effectivePlanAmount(plan)
  return Number.isInteger(amount) ? String(amount) : amount.toFixed(2)
}

function planPriceSuffix(plan: SubscriptionPlan): string {
  if (plan.purchase_quote?.action === 'extend') return `/${plan.validity_days}天延期`
  if (plan.validity_days === 1) return '/天'
  return `/${plan.validity_days}天`
}

function planBadge(plan: SubscriptionPlan): string {
  if (plan.validity_days === 1) return '临时冲刺'
  if (plan.validity_days <= 7) return '一周开发'
  return '长期高频'
}

function planTagline(plan: SubscriptionPlan): string {
  if (plan.validity_days === 1) return '今天要赶项目，就别买一个月。'
  if (plan.validity_days <= 7) return '一周持续写代码，多数人选这个。'
  return '每天都用 AI 编程，月卡最省心。'
}

function planValueLine(plan: SubscriptionPlan): string {
  if (plan.validity_days === 1) return '适合 Debug、临时冲刺、跑一次 Agent 工作流。'
  if (plan.validity_days <= 7) return `日均约 ¥${dailyAverage(plan)}，比天天买日卡省 ${dailySavingPercent(plan)}%。`
  return `日均约 ¥${dailyAverage(plan)}，比天天买日卡省 ${dailySavingPercent(plan)}%。`
}

function planBullets(plan: SubscriptionPlan): string[] {
  if (plan.validity_days === 1) return ['当天高强度使用', '买完立即生效', '不用承担长期订阅']
  if (plan.validity_days <= 7) return ['连续开发更稳', '日均成本更低', '适合大多数开发者']
  return ['长期项目更省心', '每天打开就能用', '日均最低']
}

function dailyAverage(plan: SubscriptionPlan): string {
  const days = Math.max(1, Number(plan.validity_days || 1))
  return (Number(plan.price || 0) / days).toFixed(2)
}

function dailySavingPercent(plan: SubscriptionPlan): number {
  const base = 9.9
  const avg = Number(plan.price || 0) / Math.max(1, Number(plan.validity_days || 1))
  return Math.max(0, Math.round((1 - avg / base) * 100))
}

function planDeductBalance(plan: SubscriptionPlan): string {
  return (effectivePlanAmount(plan) * safeMultiplier.value).toFixed(0)
}

function planButtonText(plan: SubscriptionPlan): string {
  if (plan.purchase_quote?.action === 'extend') return '余额延期'
  return '余额购买'
}

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Selected method's limits (for validation and error messages)
const _selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])

// Auto-switch to first available method when current selection can't handle the amount
watch(() => [validAmount.value, selectedMethod.value] as const, ([amt, method]) => {
  if (amt <= 0 || amountFitsMethod(amt, method)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(amt, m))
  if (available) selectedMethod.value = available
})

function _selectPlan(plan: SubscriptionPlan) {
  selectedPlan.value = plan
  errorMessage.value = ''
  errorHintMessage.value = ''
}

async function purchaseSelectedPlanWithBalance() {
  if (!selectedPlan.value) return
  openBalancePurchaseConfirm(selectedPlan.value)
}

function openBalancePurchaseConfirm(plan: SubscriptionPlan) {
  if (!plan || submitting.value) return
  selectedPlan.value = plan
  pendingBalancePlan.value = plan
  showBalancePurchaseConfirm.value = true
}

function cancelBalancePurchase() {
  showBalancePurchaseConfirm.value = false
  pendingBalancePlan.value = null
}

async function confirmBalancePurchase() {
  const plan = pendingBalancePlan.value
  showBalancePurchaseConfirm.value = false
  pendingBalancePlan.value = null
  if (!plan) return
  await executeBalancePurchase(plan)
}

async function purchasePlanWithBalance(plan: SubscriptionPlan) {
  openBalancePurchaseConfirm(plan)
}

async function rechargeWithAlipay() {
  const amt = rechargeAmount.value
  if (amt <= 0) {
    appStore.showError('请选择或输入充值金额')
    return
  }
  if (!amountFitsMethod(amt, 'alipay')) {
    appStore.showError('当前金额不在支付宝支付范围内')
    return
  }
  amount.value = amt
  selectedMethod.value = 'alipay'
  checkoutPlan.value = null
  checkoutMode.value = 'recharge'
}

async function purchasePlanWithAlipay(plan: SubscriptionPlan) {
  if (!plan || submitting.value) return
  selectedPlan.value = null
  selectedMethod.value = 'alipay'
  checkoutPlan.value = plan
  checkoutMode.value = 'subscription'
}

function closeCheckout() {
  checkoutMode.value = null
  checkoutPlan.value = null
}

async function confirmAlipayCheckout() {
  if (checkoutMode.value === 'subscription' && checkoutPlan.value) {
    await createOrder(effectivePlanAmount(checkoutPlan.value), 'subscription', checkoutPlan.value.id, { paymentType: 'alipay' })
    return
  }
  const amt = rechargeAmount.value
  await createOrder(amt, 'balance', undefined, { paymentType: 'alipay' })
}

async function executeBalancePurchase(plan: SubscriptionPlan) {
  if (!plan || submitting.value) return
  selectedPlan.value = plan
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  try {
    const result = await paymentStore.purchaseSubscriptionWithBalance(plan.id)
    await Promise.all([
      authStore.refreshUser().catch(() => {}),
      subscriptionStore.fetchActiveSubscriptions(true).catch(() => {}),
    ])
    selectedPlan.value = null
    appStore.showSuccess?.(`${planButtonText(plan)}成功，已扣除 ¥${Number(result.amount || effectivePlanAmount(plan)).toFixed(2)}`)
  } catch (err: unknown) {
    const metadata = (err && typeof err === 'object' && 'metadata' in err) ? (err as any).metadata : null
    if ((err as any)?.reason === 'INSUFFICIENT_BALANCE') {
      const balance = metadata?.cash_balance ?? metadata?.balance ?? cashBalanceDisplay.value
      const required = metadata?.required ?? effectivePlanAmount(plan).toFixed(2)
      errorMessage.value = `充值余额不足：当前充值余额 $${balance}，套餐需要 $${required}`
      errorHintMessage.value = '赠送余额不能购买套餐，请先使用兑换码或支付宝充值后再购买。'
    } else {
      errorMessage.value = extractApiErrorMessage(err) || '购买失败'
      errorHintMessage.value = ''
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  try {
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      paymentType: requestType,
      orderType,
      planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
    })
    if (options.openid) {
      payload.openid = options.openid
    }
    if (options.wechatResumeToken) {
      payload.wechat_resume_token = options.wechatResumeToken
    }

    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) {
        window.location.href = url
      }
    }
    const visibleMethod = normalizeVisibleMethod(requestType) || requestType
    // When user clicks the dedicated Stripe button, leave method blank so the
    // landing page renders Stripe's full Payment Element (card/link/alipay/wxpay).
    const stripeMethod = visibleMethod === 'stripe'
      ? ''
      : visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
    })

    if (decision.kind === 'wechat_oauth' && decision.oauth?.authorize_url) {
      window.location.href = buildWechatOAuthAuthorizeUrl(decision.oauth.authorize_url, {
        paymentType: visibleMethod,
        orderType,
        planId,
        orderAmount,
      })
      return
    }

    if (decision.kind === 'unhandled') {
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    if (decision.kind === 'stripe_popup') {
      openWindow(decision.paymentState.payUrl)
      return
    }
    if (decision.kind === 'stripe_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'wechat_jsapi' && decision.jsapi) {
      try {
        const jsapiResult = await invokeWechatJsapiPayment(decision.jsapi as Record<string, unknown>)
        const errMsg = String(jsapiResult.err_msg || '').toLowerCase()
        if (errMsg.includes('cancel')) {
          appStore.showInfo(t('payment.qr.cancelled'))
          resetPayment()
        } else if (errMsg && !errMsg.includes('ok')) {
          resetPayment()
          const fallbackApplied = await attemptMobileQrFallback(
            { reason: 'WECHAT_JSAPI_FAILED', message: errMsg },
            {
              orderAmount,
              orderType,
              planId,
              paymentType: visibleMethod,
              attempted: options.mobileQrFallbackAttempted === true,
            },
          )
          if (!fallbackApplied) {
            applyScenarioError({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, visibleMethod)
          }
        } else {
          const resultState = { ...decision.paymentState }
          resetPayment()
          await redirectToPaymentResult(resultState)
        }
      } catch (err: unknown) {
        resetPayment()
        const fallbackApplied = await attemptMobileQrFallback(err, {
          orderAmount,
          orderType,
          planId,
          paymentType: visibleMethod,
          attempted: options.mobileQrFallbackAttempted === true,
        })
        if (!fallbackApplied) {
          throw err
        }
      }
      return
    }
    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      if (isMobileDevice()) {
        window.location.href = decision.paymentState.payUrl
        return
      }
      openWindow(decision.paymentState.payUrl)
    }
  } catch (err: unknown) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      errorMessage.value = t('payment.errors.tooManyPending', { max: metadata?.max || '' })
      errorHintMessage.value = ''
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
      errorHintMessage.value = ''
    } else if (await attemptMobileQrFallback(err, {
      orderAmount,
      orderType,
      planId,
      paymentType: requestType,
      attempted: options.mobileQrFallbackAttempted === true,
    })) {
      return
    } else {
      const handled = applyScenarioError(
        err,
        normalizeVisibleMethod(options.paymentType || selectedMethod.value) || selectedMethod.value,
      )
      if (!handled) {
        errorMessage.value = extractI18nErrorMessage(err, t, 'payment.errors', extractApiErrorMessage(err, t('payment.result.failed')))
        errorHintMessage.value = ''
      }
      if (handled) {
        return
      }
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
}

interface MobileQrFallbackContext {
  orderAmount: number
  orderType: OrderType
  planId?: number
  paymentType: string
  attempted: boolean
}

function shouldFallbackToDesktopQr(err: unknown, paymentMethod: string, attempted: boolean): boolean {
  if (attempted || !isMobileDevice()) {
    return false
  }

  const normalizedMethod = normalizeVisibleMethod(paymentMethod) || paymentMethod
  const reason = typeof err === 'object' && err && 'reason' in err && typeof err.reason === 'string'
    ? err.reason
    : ''
  const message = err instanceof Error
    ? err.message
    : (typeof err === 'object' && err && 'message' in err && typeof err.message === 'string'
      ? err.message
      : '')
  const normalizedMessage = message.toLowerCase()

  if (normalizedMethod === 'wxpay') {
    return reason === 'WECHAT_H5_NOT_AUTHORIZED'
      || reason === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED'
      || reason === 'WECHAT_JSAPI_FAILED'
      || reason === 'PAYMENT_GATEWAY_ERROR'
      || reason === 'UNHANDLED_PAYMENT_SCENARIO'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
  }

  if (normalizedMethod === 'alipay') {
    return reason === 'PAYMENT_GATEWAY_ERROR' || reason === 'UNHANDLED_PAYMENT_SCENARIO'
  }

  return false
}

async function attemptMobileQrFallback(err: unknown, context: MobileQrFallbackContext): Promise<boolean> {
  if (!shouldFallbackToDesktopQr(err, context.paymentType, context.attempted)) {
    return false
  }

  try {
    const visibleMethod = normalizeVisibleMethod(context.paymentType) || context.paymentType
    const payload = buildCreateOrderPayload({
      amount: context.orderAmount,
      paymentType: visibleMethod,
      orderType: context.orderType,
      planId: context.planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: false,
      isWechatBrowser: false,
    })
    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const stripeMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType: context.orderType,
      isMobile: false,
      isWechatBrowser: false,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
    })

    if (decision.kind !== 'qr_waiting' || !decision.paymentState.qrCode) {
      return false
    }

    errorMessage.value = ''
    errorHintMessage.value = ''
    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)
    appStore.showWarning(t('payment.errors.mobilePaymentFallbackToQr'))
    return true
  } catch {
    return false
  }
}

function applyScenarioError(err: unknown, paymentMethod: string): boolean {
  const descriptor = describePaymentScenarioError(err, {
    paymentMethod,
    isMobile: isMobileDevice(),
    isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
  })
  if (!descriptor) {
    errorMessage.value = ''
    errorHintMessage.value = ''
    return false
  }
  errorMessage.value = t(descriptor.messageKey)
  errorHintMessage.value = descriptor.hintKey ? t(descriptor.hintKey) : ''
  appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  return true
}

async function resumeWechatPaymentFromQuery() {
  const resume = parseWechatResumeRoute(route.query, checkout.value.plans, validAmount.value)
  if (!resume) {
    return
  }

  selectedMethod.value = resume.paymentType
  if (resume.orderType === 'balance' && resume.orderAmount > 0) {
    amount.value = resume.orderAmount
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
  }

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      isResume: true,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      isResume: true,
    })
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    if (enabledMethods.value.length) {
      const order: readonly string[] = METHOD_ORDER
      const sorted = [...enabledMethods.value].sort((a, b) => {
        const ai = order.indexOf(a)
        const bi = order.indexOf(b)
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      })
      selectedMethod.value = sorted[0]
    }
    if (typeof window !== 'undefined') {
      if (hasWechatResumeQuery(route.query)) {
        removeRecoverySnapshot()
      }
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : typeof route.query.wechat_resume_token === 'string'
          ? route.query.wechat_resume_token
          : undefined
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken: routeResumeToken },
      )
      if (restored) {
        paymentState.value = restored
        paymentPhase.value = 'paying'
        const restoredMethod = normalizeVisibleMethod(restored.paymentType)
        if (restoredMethod) {
          selectedMethod.value = restoredMethod
        }
      } else {
        removeRecoverySnapshot()
      }
    }
    await resumeWechatPaymentFromQuery()
    if (checkout.value.balance_disabled) {
      activeTab.value = 'subscription'
    }
    // Handle renewal navigation: ?tab=subscription&group=123
    if (route.query.tab === 'subscription') {
      activeTab.value = 'subscription'
      if (route.query.group) {
        const groupId = Number(route.query.group)
        const groupPlans = checkout.value.plans.filter(p => p.group_id === groupId)
        if (groupPlans.length === 1) {
          selectedPlan.value = groupPlans[0]
        }
      }
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
  // Fetch active subscriptions (uses cache, non-blocking)
  subscriptionStore.fetchActiveSubscriptions().catch(() => {})
})
</script>
