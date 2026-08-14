import {
	AddressLabel,
	BridgeLifecycle,
	CrossChainTransition,
	EntityType,
	GraphEdge,
	GraphNode,
	InvestigationLead,
	LabelVisibility,
	type SupportedNetwork,
	TraceGraphResponse,
	networkFromSlug,
} from './api';
import { type LocalCase, parseCaseFile } from './case-file';

export const EVIDENCE_PACKAGE_FORMAT = 'openchain-evidence-package';
export const EVIDENCE_PACKAGE_VERSION = 3;

type FrozenTransfer = {
	id: string;
	network: string;
	transaction_hash: string;
	event_id: string;
	transfer_kind: string;
	from_address: string;
	to_address: string;
	asset: { kind: string; contract_address: string; symbol: string; decimals: number };
	amount_base_units: string;
	block_number: number;
	block_hash: string;
	block_timestamp: string;
	provisional: boolean;
	source: string;
	retrieved_at: string;
};

type FrozenLabel = {
	id: string;
	network: string;
	address: string;
	category: string;
	label: string;
	confidence: number;
	evidence_url: string;
	source: string;
	source_version: string;
	visibility: string;
	trust_tier: number;
	created_by: string;
	created_at: string;
};

type FrozenRuleRun = {
	rule_id: string;
	rule_version: string;
	parameters: unknown;
	result: unknown;
};

type FrozenBridgeTransition = {
	id: string;
	protocol: string;
	bridge_name: string;
	source_network: string;
	destination_network: string;
	lifecycle: string;
	message_id: string;
	source_transfer_id: string;
	destination_transfer_id: string;
	source_transaction_hash: string;
	destination_transaction_hash: string;
	source_log_reference: string;
	destination_log_reference: string;
	source_bridge_address: string;
	destination_bridge_address: string;
	canonical_source_token: string;
	canonical_destination_token: string;
	recipient: string;
	asset: { kind: string; contract_address: string; symbol: string; decimals: number };
	amount_base_units: string;
	source_block_number: number;
	destination_block_number: number;
	source_block_hash: string;
	destination_block_hash: string;
	source_timestamp: string;
	destination_timestamp: string;
	source_confirmed: boolean;
	destination_confirmed: boolean;
	limitations: string;
};

type FrozenBridgeTransitionAcquisition = {
	transition_id: string;
	side: 'source' | 'destination';
	acquisition_id: number;
};

export type EvidencePackage = {
	format: string;
	version: number;
	payload: {
		exported_at: string;
		case: LocalCase;
		transfers: FrozenTransfer[];
		acquisition_snapshots: unknown[];
		acquisition_scopes: unknown[];
		scope_transfers: unknown[];
		scope_snapshots: unknown[];
		rule_runs: FrozenRuleRun[];
		labels: FrozenLabel[];
		bridge_transitions: FrozenBridgeTransition[];
		bridge_transition_acquisitions: FrozenBridgeTransitionAcquisition[];
	};
	manifest: { algorithm: string; payload_sha256: string };
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === 'object' && value !== null && !Array.isArray(value);

const isTransfer = (value: unknown): value is FrozenTransfer => {
	if (!isRecord(value) || !isRecord(value.asset)) return false;
	return (
		[
			'id',
			'network',
			'transaction_hash',
			'event_id',
			'transfer_kind',
			'from_address',
			'to_address',
			'amount_base_units',
			'block_timestamp',
			'source',
			'retrieved_at',
		].every((key) => typeof value[key] === 'string') &&
		typeof value.block_number === 'number' &&
		typeof value.provisional === 'boolean' &&
		typeof value.asset.kind === 'string' &&
		typeof value.asset.contract_address === 'string' &&
		typeof value.asset.symbol === 'string' &&
		typeof value.asset.decimals === 'number'
	);
};

const isLabel = (value: unknown): value is FrozenLabel =>
	isRecord(value) &&
	[
		'id',
		'network',
		'address',
		'category',
		'label',
		'evidence_url',
		'source',
		'source_version',
		'visibility',
		'created_by',
		'created_at',
	].every((key) => typeof value[key] === 'string') &&
	typeof value.confidence === 'number' &&
	typeof value.trust_tier === 'number';

const isSnapshot = (value: unknown) =>
	isRecord(value) &&
	typeof value.id === 'number' &&
	[
		'network',
		'provider',
		'request_identity',
		'response_sha256',
		'response_body_base64',
		'retrieved_at',
	].every((key) => typeof value[key] === 'string');

const isAcquisitionScope = (value: unknown) =>
	isRecord(value) &&
	typeof value.id === 'number' &&
	['network', 'address', 'cursor', 'retrieved_at'].every((key) => typeof value[key] === 'string');

const isScopeTransfer = (value: unknown) =>
	isRecord(value) && typeof value.scope_id === 'number' && typeof value.transfer_id === 'string';

const isScopeSnapshot = (value: unknown) =>
	isRecord(value) && typeof value.scope_id === 'number' && typeof value.acquisition_id === 'number';

const isRuleRun = (value: unknown): value is FrozenRuleRun =>
	isRecord(value) &&
	typeof value.rule_id === 'string' &&
	typeof value.rule_version === 'string' &&
	'parameters' in value &&
	'result' in value;

const isBridgeTransition = (value: unknown): value is FrozenBridgeTransition =>
	isRecord(value) &&
	isRecord(value.asset) &&
	[
		'id',
		'protocol',
		'bridge_name',
		'source_network',
		'destination_network',
		'lifecycle',
		'message_id',
		'source_transfer_id',
		'destination_transfer_id',
		'source_transaction_hash',
		'destination_transaction_hash',
		'source_log_reference',
		'destination_log_reference',
		'source_bridge_address',
		'destination_bridge_address',
		'canonical_source_token',
		'canonical_destination_token',
		'recipient',
		'amount_base_units',
		'source_block_hash',
		'destination_block_hash',
		'source_timestamp',
		'destination_timestamp',
		'limitations',
	].every((key) => typeof value[key] === 'string') &&
	typeof value.source_block_number === 'number' &&
	typeof value.destination_block_number === 'number' &&
	typeof value.source_confirmed === 'boolean' &&
	typeof value.destination_confirmed === 'boolean' &&
	typeof value.asset.kind === 'string' &&
	typeof value.asset.contract_address === 'string' &&
	typeof value.asset.symbol === 'string' &&
	typeof value.asset.decimals === 'number';

const isBridgeTransitionAcquisition = (
	value: unknown,
): value is FrozenBridgeTransitionAcquisition =>
	isRecord(value) &&
	typeof value.transition_id === 'string' &&
	(value.side === 'source' || value.side === 'destination') &&
	typeof value.acquisition_id === 'number';

async function payloadHash(payload: unknown): Promise<string> {
	const digest = await crypto.subtle.digest(
		'SHA-256',
		new TextEncoder().encode(JSON.stringify(payload)),
	);
	return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, '0')).join('');
}

export async function parseEvidencePackage(value: string): Promise<EvidencePackage> {
	if (value.length > 8 << 20) throw new Error('Evidence packages must be smaller than 8 MB.');
	const parsed: unknown = JSON.parse(value);
	if (
		!isRecord(parsed) ||
		parsed.format !== EVIDENCE_PACKAGE_FORMAT ||
		parsed.version !== EVIDENCE_PACKAGE_VERSION ||
		!isRecord(parsed.payload) ||
		!isRecord(parsed.manifest) ||
		parsed.manifest.algorithm !== 'SHA-256' ||
		typeof parsed.manifest.payload_sha256 !== 'string'
	)
		throw new Error('This is not a supported OpenChain evidence package.');
	const payload = parsed.payload;
	if (
		!Array.isArray(payload.transfers) ||
		payload.transfers.length === 0 ||
		payload.transfers.length > 250 ||
		!payload.transfers.every(isTransfer) ||
		!Array.isArray(payload.acquisition_snapshots) ||
		!payload.acquisition_snapshots.every(isSnapshot) ||
		!Array.isArray(payload.acquisition_scopes) ||
		!payload.acquisition_scopes.every(isAcquisitionScope) ||
		!Array.isArray(payload.scope_transfers) ||
		!payload.scope_transfers.every(isScopeTransfer) ||
		!Array.isArray(payload.scope_snapshots) ||
		!payload.scope_snapshots.every(isScopeSnapshot) ||
		!Array.isArray(payload.labels) ||
		!payload.labels.every(isLabel) ||
		!Array.isArray(payload.rule_runs) ||
		!payload.rule_runs.every(isRuleRun) ||
		!Array.isArray(payload.bridge_transitions) ||
		!payload.bridge_transitions.every(isBridgeTransition) ||
		!Array.isArray(payload.bridge_transition_acquisitions) ||
		!payload.bridge_transition_acquisitions.every(isBridgeTransitionAcquisition) ||
		typeof payload.exported_at !== 'string'
	)
		throw new Error('Evidence package contents are invalid.');
	const transferIDs = new Set(payload.transfers.map((transfer) => transfer.id));
	const scopeIDs = new Set(payload.acquisition_scopes.map((scope) => scope.id));
	const snapshotIDs = new Set(
		payload.acquisition_snapshots.map((snapshot) => (snapshot as { id: number }).id),
	);
	const bridgeTransitionIDs = new Set(
		payload.bridge_transitions.map((transition) => transition.id),
	);
	if (
		payload.scope_transfers.some(
			(link) =>
				!scopeIDs.has((link as { scope_id: number }).scope_id) ||
				!transferIDs.has((link as { transfer_id: string }).transfer_id),
		) ||
		payload.scope_snapshots.some(
			(link) =>
				!scopeIDs.has((link as { scope_id: number }).scope_id) ||
				!snapshotIDs.has((link as { acquisition_id: number }).acquisition_id),
		) ||
		payload.bridge_transitions.some(
			(transition) => !transferIDs.has(transition.source_transfer_id),
		) ||
		payload.bridge_transition_acquisitions.some(
			(link) =>
				!bridgeTransitionIDs.has(link.transition_id) || !snapshotIDs.has(link.acquisition_id),
		)
	)
		throw new Error('Evidence package scope links are inconsistent.');
	const caseFile = parseCaseFile(JSON.stringify(payload.case));
	if (
		caseFile.network !== payload.transfers[0].network ||
		payload.transfers.some((transfer) => transfer.network !== caseFile.network)
	)
		throw new Error('Evidence package network is inconsistent.');
	if ((await payloadHash(payload)) !== parsed.manifest.payload_sha256.toLowerCase())
		throw new Error('Evidence package integrity check failed.');
	return parsed as EvidencePackage;
}

const unix = (value: string) => {
	const time = Date.parse(value);
	if (Number.isNaN(time)) throw new Error('Evidence package timestamp is invalid.');
	return BigInt(Math.floor(time / 1000));
};

const network = (value: string): SupportedNetwork =>
	networkFromSlug(value as Parameters<typeof networkFromSlug>[0]);

function frozenLeads(runs: readonly FrozenRuleRun[]): InvestigationLead[] {
	const leads: InvestigationLead[] = [];
	for (const run of runs) {
		if (!Array.isArray(run.result)) continue;
		for (const item of run.result) {
			if (
				!isRecord(item) ||
				typeof item.id !== 'string' ||
				typeof item.title !== 'string' ||
				typeof item.subject_address !== 'string' ||
				!Array.isArray(item.transfer_ids) ||
				!item.transfer_ids.every((id) => typeof id === 'string')
			)
				continue;
			leads.push(
				new InvestigationLead({
					id: item.id,
					ruleId: run.rule_id,
					ruleVersion: run.rule_version,
					title: item.title,
					subjectAddress: item.subject_address,
					transferIds: item.transfer_ids,
					rationale: typeof item.rationale === 'string' ? item.rationale : '',
					limitations: typeof item.limitations === 'string' ? item.limitations : '',
					parametersJson: JSON.stringify(run.parameters),
				}),
			);
		}
	}
	return leads;
}

const lifecycle = (value: string): BridgeLifecycle => {
	switch (value) {
		case 'initiated':
			return BridgeLifecycle.INITIATED;
		case 'relayed':
			return BridgeLifecycle.RELAYED;
		case 'finalized':
			return BridgeLifecycle.FINALIZED;
		case 'failed':
			return BridgeLifecycle.FAILED;
		default:
			return BridgeLifecycle.UNRESOLVED;
	}
};

function frozenBridgeTransitions(items: readonly FrozenBridgeTransition[]): CrossChainTransition[] {
	return items.map(
		(item) =>
			new CrossChainTransition({
				id: item.id,
				protocol: item.protocol,
				bridgeName: item.bridge_name,
				sourceNetwork: network(item.source_network),
				destinationNetwork: network(item.destination_network),
				lifecycle: lifecycle(item.lifecycle),
				messageId: item.message_id,
				sourceTransferId: item.source_transfer_id,
				destinationTransferId: item.destination_transfer_id,
				sourceTransactionHash: item.source_transaction_hash,
				destinationTransactionHash: item.destination_transaction_hash,
				sourceLogReference: item.source_log_reference,
				destinationLogReference: item.destination_log_reference,
				sourceBridgeAddress: item.source_bridge_address,
				destinationBridgeAddress: item.destination_bridge_address,
				canonicalSourceToken: item.canonical_source_token,
				canonicalDestinationToken: item.canonical_destination_token,
				recipient: item.recipient,
				asset: item.asset,
				amountBaseUnits: item.amount_base_units,
				sourceBlockNumber: BigInt(item.source_block_number),
				destinationBlockNumber: BigInt(item.destination_block_number),
				sourceTimestamp: unix(item.source_timestamp),
				destinationTimestamp: unix(item.destination_timestamp),
				sourceConfirmed: item.source_confirmed,
				destinationConfirmed: item.destination_confirmed,
				limitations: item.limitations,
			}),
	);
}

export function replayEvidencePackage(packageFile: EvidencePackage): {
	caseFile: LocalCase;
	network: SupportedNetwork;
	graph: TraceGraphResponse;
} {
	const caseFile = parseCaseFile(JSON.stringify(packageFile.payload.case));
	const targetNetwork = network(caseFile.network);
	const labelsByAddress = new Map<string, AddressLabel[]>();
	for (const label of packageFile.payload.labels) {
		const items = labelsByAddress.get(label.address) || [];
		items.push(
			new AddressLabel({
				id: label.id,
				address: label.address,
				network: targetNetwork,
				category: label.category,
				label: label.label,
				confidence: label.confidence,
				evidenceUrl: label.evidence_url,
				source: label.source,
				sourceVersion: label.source_version,
				visibility:
					label.visibility === 'public' ? LabelVisibility.PUBLIC : LabelVisibility.UNSPECIFIED,
				createdBy: label.created_by,
				createdAt: unix(label.created_at),
				trustTier: label.trust_tier,
			}),
		);
		labelsByAddress.set(label.address, items);
	}
	const nodes = new Map<string, GraphNode>();
	const edges = packageFile.payload.transfers.map((transfer) => {
		for (const address of [transfer.from_address, transfer.to_address]) {
			if (!nodes.has(address))
				nodes.set(
					address,
					new GraphNode({
						id: address,
						label: address,
						entityType: EntityType.EOA,
						isSeed: address === caseFile.rootAddress,
						labels: labelsByAddress.get(address) || [],
					}),
				);
		}
		return new GraphEdge({
			id: transfer.id,
			source: transfer.from_address,
			target: transfer.to_address,
			amountBaseUnits: transfer.amount_base_units,
			amountFormatted: `${transfer.amount_base_units} ${transfer.asset.symbol}`,
			txCount: 1,
			asset: {
				kind: transfer.asset.kind,
				contractAddress: transfer.asset.contract_address,
				symbol: transfer.asset.symbol,
				decimals: transfer.asset.decimals,
			},
			eventId: transfer.event_id,
			blockNumber: BigInt(transfer.block_number),
			transactionHash: transfer.transaction_hash,
			transferKind: transfer.transfer_kind,
			sourceName: transfer.source,
			retrievedAt: unix(transfer.retrieved_at),
			firstTxTimestamp: unix(transfer.block_timestamp),
			lastTxTimestamp: unix(transfer.block_timestamp),
			provisional: transfer.provisional,
		});
	});
	return {
		caseFile,
		network: targetNetwork,
		graph: new TraceGraphResponse({
			seedAddress: caseFile.rootAddress,
			nodes: [...nodes.values()],
			edges,
			totalNodes: nodes.size,
			totalEdges: edges.length,
			sourceStatus: {
				source: 'Frozen evidence package',
				retrievedAt: unix(packageFile.payload.exported_at),
				isComplete: true,
			},
			leads: frozenLeads(packageFile.payload.rule_runs),
			crossChainTransitions: frozenBridgeTransitions(packageFile.payload.bridge_transitions),
		}),
	};
}

export function evidencePrintHTML(packageFile: EvidencePackage): string {
	const { caseFile, graph } = replayEvidencePackage(packageFile);
	const escapeHTML = (value: string) =>
		value
			.replaceAll('&', '&amp;')
			.replaceAll('<', '&lt;')
			.replaceAll('>', '&gt;')
			.replaceAll('"', '&quot;');
	return `<!doctype html><title>${escapeHTML(caseFile.title)}</title><style>body{font:14px system-ui;margin:40px;color:#1a1d23}small,p{color:#4b5068}article{white-space:pre-wrap}table{border-collapse:collapse;width:100%;font-size:12px}td,th{border:1px solid #ddd;padding:6px;text-align:left}</style><h1>${escapeHTML(caseFile.title)}</h1><small>Frozen OpenChain evidence package · ${escapeHTML(caseFile.network)} · ${escapeHTML(packageFile.manifest.payload_sha256)}</small><h2>Notes</h2><article>${escapeHTML(caseFile.notes)}</article><h2>Transfers</h2><table><tr><th>Transaction</th><th>From</th><th>To</th><th>Amount</th><th>Confirmation</th><th>Source</th></tr>${graph.edges.map((edge) => `<tr><td>${escapeHTML(edge.transactionHash)}</td><td>${escapeHTML(edge.source)}</td><td>${escapeHTML(edge.target)}</td><td>${escapeHTML(edge.amountBaseUnits)} ${escapeHTML(edge.asset?.symbol || '')}</td><td>${edge.provisional ? 'Provisional' : 'Confirmation-backed'}</td><td>${escapeHTML(edge.sourceName)}</td></tr>`).join('')}</table><h2>Investigation leads</h2><ul>${graph.leads.map((lead) => `<li><strong>${escapeHTML(lead.title)}</strong> — ${escapeHTML(lead.rationale)}<br><small>${escapeHTML(lead.limitations)}</small></li>`).join('')}</ul>`;
}
