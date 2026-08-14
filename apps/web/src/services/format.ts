export function formatObservationTime(timestamp: bigint): string {
	const date = new Date(Number(timestamp) * 1000);
	if (!Number.isFinite(date.getTime())) return 'Unknown observation time';
	return new Intl.DateTimeFormat(undefined, {
		year: 'numeric',
		month: 'short',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		timeZone: 'UTC',
		timeZoneName: 'short',
	}).format(date);
}
