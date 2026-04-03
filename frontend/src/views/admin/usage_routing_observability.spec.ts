import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

describe('admin usage routing observability wiring', () => {
	it('extends usage and ops request detail types with routing fields', () => {
		const typesSource = readFileSync(resolve(__dirname, '../../types/index.ts'), 'utf8')
		const opsApiSource = readFileSync(resolve(__dirname, '../../api/admin/ops.ts'), 'utf8')

		expect(typesSource).toContain('routing_target_group?: string | null')
		expect(typesSource).toContain('routing_schedule_layer?: string | null')
		expect(typesSource).toContain('routing_selected_account_name?: string | null')
		expect(typesSource).toContain('routing_effective_model?: string | null')
		expect(typesSource).toContain('routing_failover_count?: number | null')
		expect(typesSource).toContain('routing_failover_final_reason?: string | null')
		expect(opsApiSource).toContain('routing_target_group?: string')
		expect(opsApiSource).toContain('routing_schedule_layer?: string')
		expect(opsApiSource).toContain('routing_selected_account_name?: string | null')
	})

	it('wires routing filters, columns, and export fields into admin usage view', () => {
		const usageViewSource = readFileSync(resolve(__dirname, './UsageView.vue'), 'utf8')
		const usageFiltersSource = readFileSync(resolve(__dirname, '../../components/admin/usage/UsageFilters.vue'), 'utf8')
		const usageTableSource = readFileSync(resolve(__dirname, '../../components/admin/usage/UsageTable.vue'), 'utf8')

		expect(usageViewSource).toContain("routing_target_group")
		expect(usageViewSource).toContain("routing_schedule_layer")
		expect(usageViewSource).toContain("routing_selected_account_name")
		expect(usageViewSource).toContain("routing_effective_model")
		expect(usageViewSource).toContain("routing_failover_count")
		expect(usageFiltersSource).toContain('filters.routing_target_group')
		expect(usageFiltersSource).toContain('filters.routing_schedule_layer')
		expect(usageTableSource).toContain('cell-routing_target_group')
		expect(usageTableSource).toContain('cell-routing_schedule_layer')
		expect(usageTableSource).toContain('cell-routing_selected_account_name')
	})

	it('shows routing fields inside ops request details modal', () => {
		const modalSource = readFileSync(resolve(__dirname, './ops/components/OpsRequestDetailsModal.vue'), 'utf8')

		expect(modalSource).toContain('row.routing_target_group')
		expect(modalSource).toContain('row.routing_schedule_layer')
		expect(modalSource).toContain('row.routing_selected_account_name')
		expect(modalSource).toContain('row.routing_effective_model')
		expect(modalSource).toContain('row.routing_failover_count')
	})
})
