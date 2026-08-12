import { InvestigationLead } from '@openchain/proto/openchain/v1/tracing_pb';
import { describe, expect, it } from 'vitest';
import { leadThreshold } from './InvestigationLeads';

describe('leadThreshold', () => {
	it('shows a rule threshold without interpreting the lead as risk', () => {
		expect(
			leadThreshold(
				new InvestigationLead({
					parametersJson: '{"window_seconds":86400,"min_distinct_counterparties":3}',
				}),
			),
		).toBe('24h window · 3 counterparties');
	});
});
