import { Download, FileUp, Printer, Save } from 'lucide-react';
import type React from 'react';
import { useRef, useState } from 'react';
import type { LocalCase } from '../services/case-file';
import {
	type EvidencePackage,
	evidencePrintHTML,
	parseEvidencePackage,
} from '../services/evidence-package';

type Props = {
	caseFile: LocalCase;
	activeTarget: { kind: 'address' | 'transfer'; id: string } | null;
	onChange: (caseFile: LocalCase) => void;
	onExportEvidence: () => Promise<string>;
	onImportEvidence: (packageFile: EvidencePackage) => void;
	frozenEvidence: EvidencePackage | null;
};

const download = (name: string, content: string, type: string) => {
	const link = document.createElement('a');
	link.href = URL.createObjectURL(new Blob([content], { type }));
	link.download = name;
	link.click();
	URL.revokeObjectURL(link.href);
};

export const CaseWorkspace: React.FC<Props> = ({
	caseFile,
	activeTarget,
	onChange,
	onExportEvidence,
	onImportEvidence,
	frozenEvidence,
}) => {
	const fileInput = useRef<HTMLInputElement>(null);
	const [annotation, setAnnotation] = useState('');
	const [error, setError] = useState('');
	const [exporting, setExporting] = useState(false);
	const update = (change: Partial<LocalCase>) =>
		onChange({ ...caseFile, ...change, updatedAt: new Date().toISOString() });
	const addAnnotation = () => {
		if (!activeTarget || !annotation.trim()) return;
		update({
			annotations: [
				...caseFile.annotations,
				{
					id: crypto.randomUUID(),
					target: activeTarget,
					note: annotation.trim(),
					createdAt: new Date().toISOString(),
				},
			],
		});
		setAnnotation('');
	};
	const exportPackage = async () => {
		setExporting(true);
		try {
			const content = await onExportEvidence();
			await parseEvidencePackage(content);
			download('openchain-evidence-package.json', content, 'application/json');
			setError('');
		} catch (cause) {
			setError(cause instanceof Error ? cause.message : 'Unable to create an evidence package.');
		} finally {
			setExporting(false);
		}
	};
	const importFile = async (file: File | undefined) => {
		if (!file) return;
		if (file.size > 8 << 20) {
			setError('Evidence packages must be smaller than 8 MB.');
			return;
		}
		try {
			onImportEvidence(await parseEvidencePackage(await file.text()));
			setError('');
		} catch {
			setError('This evidence package is invalid or has been altered.');
		}
	};
	const printable = () => {
		if (!frozenEvidence) return;
		const page = window.open('', '_blank');
		if (!page) return;
		page.document.write(evidencePrintHTML(frozenEvidence));
		page.document.close();
		page.focus();
		page.print();
	};

	return (
		<section className="space-y-3 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
			<div className="flex items-center justify-between">
				<h3
					className="text-[10px] uppercase font-bold tracking-widest"
					style={{ color: 'var(--ink-3)' }}
				>
					Local case
				</h3>
				<span className="text-[9px] uppercase font-semibold" style={{ color: '#059669' }}>
					<Save className="inline w-3 h-3 mr-1" />
					Saved here
				</span>
			</div>
			<input
				aria-label="Case title"
				value={caseFile.title}
				onChange={(event) => update({ title: event.target.value })}
				className="prism-input text-xs font-medium"
			/>
			<textarea
				aria-label="Case notes"
				value={caseFile.notes}
				onChange={(event) => update({ notes: event.target.value })}
				placeholder="Investigation notes…"
				rows={4}
				className="prism-input text-xs resize-y"
			/>
			<div className="space-y-1.5">
				<input
					aria-label="Annotation"
					value={annotation}
					onChange={(event) => setAnnotation(event.target.value)}
					placeholder={
						activeTarget
							? `Annotate ${activeTarget.kind}`
							: 'Select an address or transfer to annotate'
					}
					disabled={!activeTarget}
					className="prism-input text-xs"
				/>
				<button
					type="button"
					onClick={addAnnotation}
					disabled={!activeTarget || !annotation.trim()}
					className="btn-outline w-full text-[11px] py-1.5 disabled:opacity-50"
				>
					Add annotation
				</button>
			</div>
			{caseFile.annotations.length > 0 && (
				<div className="space-y-1 rounded-lg p-2" style={{ background: 'var(--slate)' }}>
					<p className="text-[9px] uppercase font-bold" style={{ color: 'var(--ink-3)' }}>
						Investigator annotations · local only
					</p>
					<p className="text-[9px]" style={{ color: 'var(--ink-3)' }}>
						Not verified entity labels or rule-derived findings.
					</p>
					{caseFile.annotations.slice(-3).map((item) => (
						<p key={item.id} className="text-[10px]" style={{ color: 'var(--ink-2)' }}>
							<span className="font-mono">{item.target.kind}</span> · {item.note}
						</p>
					))}
					{caseFile.annotations.length > 3 && (
						<p className="text-[9px]" style={{ color: 'var(--ink-3)' }}>
							Showing the 3 most recent of {caseFile.annotations.length} annotations.
						</p>
					)}
				</div>
			)}
			<div className="grid grid-cols-3 gap-2">
				<button
					type="button"
					onClick={() => void exportPackage()}
					disabled={exporting}
					className="btn-outline text-[11px] py-1.5"
				>
					<Download className="w-3.5 h-3.5" />
					{exporting ? 'Freezing…' : 'Package'}
				</button>
				<button
					type="button"
					onClick={() => fileInput.current?.click()}
					className="btn-outline text-[11px] py-1.5"
				>
					<FileUp className="w-3.5 h-3.5" />
					Import
				</button>
				<button
					type="button"
					onClick={printable}
					disabled={!frozenEvidence}
					className="btn-outline text-[11px] py-1.5 disabled:opacity-50"
				>
					<Printer className="w-3.5 h-3.5" />
					Print
				</button>
			</div>
			<input
				ref={fileInput}
				type="file"
				accept="application/json"
				className="hidden"
				onChange={(event) => void importFile(event.target.files?.[0])}
			/>
			{error && (
				<p className="text-[10px]" style={{ color: '#b91c1c' }}>
					{error}
				</p>
			)}
		</section>
	);
};
