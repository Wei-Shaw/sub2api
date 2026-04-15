import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

describe('admin usage routing observability wiring', () => {
	it('extends usage and ops request detail types with routing fields', () => {
		const typesSource = readFileSync(resolve(__dirname, '../../types/index.ts'), 'utf8')
		const opsApiSource = readFileSync(resolve(__dirname, '../../api/admin/ops.ts'), 'utf8')

		expect(typesSource).toContain('routing_target_group?: string | null')
		expect(typesSource).toContain('routing_selected_group?: string | null')
		expect(typesSource).toContain('routing_schedule_layer?: string | null')
		expect(typesSource).toContain('routing_selected_account_name?: string | null')
		expect(typesSource).toContain('routing_effective_model?: string | null')
		expect(typesSource).toContain('routing_failover_count?: number | null')
		expect(typesSource).toContain('routing_failover_final_reason?: string | null')
		expect(opsApiSource).toContain('routing_target_group?: string')
		expect(opsApiSource).toContain('routing_selected_group?: string | null')
		expect(opsApiSource).toContain('routing_selected_group?: string')
		expect(opsApiSource).toContain('routing_schedule_layer?: string')
		expect(opsApiSource).toContain('routing_selected_account_name?: string | null')
		expect(opsApiSource).toContain('selected_group_count: Record<string, number>')
	})

	it('wires routing filters, columns, and export fields into admin usage view', () => {
		const usageViewSource = readFileSync(resolve(__dirname, './UsageView.vue'), 'utf8')
		const usageFiltersSource = readFileSync(resolve(__dirname, '../../components/admin/usage/UsageFilters.vue'), 'utf8')
		const usageApiSource = readFileSync(resolve(__dirname, '../../api/admin/usage.ts'), 'utf8')
		const usageTableSource = readFileSync(resolve(__dirname, '../../components/admin/usage/UsageTable.vue'), 'utf8')

		expect(usageViewSource).toContain("routing_target_group")
		expect(usageViewSource).toContain("routing_selected_group")
		expect(usageViewSource).toContain("routing_schedule_layer")
		expect(usageViewSource).toContain("routing_selected_account_name")
		expect(usageViewSource).toContain("routing_effective_model")
		expect(usageViewSource).toContain("routing_failover_count")
		expect(usageFiltersSource).toContain('filters.routing_target_group')
		expect(usageFiltersSource).toContain('routingSelectedGroupHint')
		expect(usageFiltersSource).toContain('filters.routing_schedule_layer')
		expect(usageFiltersSource).toContain('filters.billing_mode')
		expect(usageApiSource).toContain('billing_mode?: string')
		expect(usageTableSource).toContain('cell-routing_target_group')
		expect(usageTableSource).toContain('cell-routing_selected_group')
		expect(usageTableSource).toContain('routingSelectedGroupReserve')
		expect(usageTableSource).toContain('cell-routing_schedule_layer')
		expect(usageTableSource).toContain('cell-routing_selected_account_name')
	})

	it('shows routing fields inside ops request details modal and uses selected-group drilldown', () => {
		const modalSource = readFileSync(resolve(__dirname, './ops/components/OpsRequestDetailsModal.vue'), 'utf8')
		const retryModalSource = readFileSync(resolve(__dirname, './ops/components/OpsOpenAIRetryDetailsModal.vue'), 'utf8')
		const retryCardSource = readFileSync(resolve(__dirname, './ops/components/OpsOpenAIRetryCard.vue'), 'utf8')
		const stickyCardSource = readFileSync(resolve(__dirname, './ops/components/OpsOpenAIStickyCard.vue'), 'utf8')
		const routingCardSource = readFileSync(resolve(__dirname, './ops/components/OpsOpenAIRoutingCard.vue'), 'utf8')
		const enSource = readFileSync(resolve(__dirname, '../../i18n/locales/en.ts'), 'utf8')
		const zhSource = readFileSync(resolve(__dirname, '../../i18n/locales/zh.ts'), 'utf8')

		expect(modalSource).toContain('row.routing_target_group')
		expect(modalSource).toContain('row.routing_selected_group')
		expect(modalSource).toContain('params.routing_selected_group')
		expect(modalSource).toContain('row.routing_schedule_layer')
		expect(modalSource).toContain('row.routing_selected_account_name')
		expect(modalSource).toContain('row.routing_effective_model')
		expect(modalSource).toContain('row.routing_failover_count')
		expect(retryModalSource).toContain('routing_selected_group')
		expect(retryCardSource).toContain('retriedRequestCountByGroup.reserve')
		expect(stickyCardSource).toContain('selected_group_count')
		expect(routingCardSource).toContain('requestCountByGroup.reserve')
		expect(enSource).toContain("routingSelectedGroupReserve: 'Reserve'")
		expect(zhSource).toContain("routingSelectedGroupReserve: 'Reserve'")
	})
})
