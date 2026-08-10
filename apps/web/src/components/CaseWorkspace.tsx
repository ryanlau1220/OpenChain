import { Download, FileDown, FileUp, Printer, Save } from 'lucide-react';
import type React from 'react';
import { useRef, useState } from 'react';
import { caseCSV, casePrintHTML, parseCaseFile, type LocalCase } from '../services/case-file';

type Props = {
	caseFile: LocalCase;
	activeTarget: { kind: 'address' | 'transfer'; id: string } | null;
	onChange: (caseFile: LocalCase) => void;
};

const download = (name: string, content: string, type: string) => {
	const link = document.createElement('a');
	link.href = URL.createObjectURL(new Blob([content], { type }));
	link.download = name;
	link.click();
	URL.revokeObjectURL(link.href);
};

export const CaseWorkspace: React.FC<Props> = ({ caseFile, activeTarget, onChange }) => {
	const fileInput = useRef<HTMLInputElement>(null);
	const [annotation, setAnnotation] = useState('');
	const [error, setError] = useState('');
	const update = (change: Partial<LocalCase>) => onChange({ ...caseFile, ...change, updatedAt: new Date().toISOString() });
	const addAnnotation = () => {
		if (!activeTarget || !annotation.trim()) return;
		update({ annotations: [...caseFile.annotations, { id: crypto.randomUUID(), target: activeTarget, note: annotation.trim(), createdAt: new Date().toISOString() }] });
		setAnnotation('');
	};
	const importFile = async (file: File | undefined) => {
		if (!file) return;
		if (file.size > 1_000_000) { setError('Case files must be smaller than 1 MB.'); return; }
		try { onChange(parseCaseFile(await file.text())); setError(''); } catch { setError('This is not a supported OpenChain case file.'); }
	};
	const printable = () => {
		const page = window.open('', '_blank');
		if (!page) return;
		page.document.write(casePrintHTML(caseFile));
		page.document.close();
		page.focus();
		page.print();
	};

	return <section className="space-y-3 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
		<div className="flex items-center justify-between">
			<h3 className="text-[10px] uppercase font-bold tracking-widest" style={{ color: 'var(--ink-3)' }}>Local case</h3>
			<span className="text-[9px] uppercase font-semibold" style={{ color: '#059669' }}><Save className="inline w-3 h-3 mr-1" />Saved here</span>
		</div>
		<input aria-label="Case title" value={caseFile.title} onChange={(event) => update({ title: event.target.value })} className="prism-input text-xs font-medium" />
		<textarea aria-label="Case notes" value={caseFile.notes} onChange={(event) => update({ notes: event.target.value })} placeholder="Investigation notes…" rows={4} className="prism-input text-xs resize-y" />
		<div className="space-y-1.5">
			<input aria-label="Annotation" value={annotation} onChange={(event) => setAnnotation(event.target.value)} placeholder={activeTarget ? `Annotate ${activeTarget.kind}` : 'Select an address or transfer to annotate'} disabled={!activeTarget} className="prism-input text-xs" />
			<button type="button" onClick={addAnnotation} disabled={!activeTarget || !annotation.trim()} className="btn-outline w-full text-[11px] py-1.5 disabled:opacity-50">Add annotation</button>
		</div>
		{caseFile.annotations.length > 0 && <p className="text-[10px]" style={{ color: 'var(--ink-3)' }}>{caseFile.annotations.length} saved annotation{caseFile.annotations.length === 1 ? '' : 's'}</p>}
		<div className="grid grid-cols-2 gap-2">
			<button type="button" onClick={() => download('openchain-case.json', JSON.stringify(caseFile, null, 2), 'application/json')} className="btn-outline text-[11px] py-1.5"><Download className="w-3.5 h-3.5" />JSON</button>
			<button type="button" onClick={() => download('openchain-case.csv', caseCSV(caseFile), 'text/csv')} className="btn-outline text-[11px] py-1.5"><FileDown className="w-3.5 h-3.5" />CSV</button>
			<button type="button" onClick={() => fileInput.current?.click()} className="btn-outline text-[11px] py-1.5"><FileUp className="w-3.5 h-3.5" />Import</button>
			<button type="button" onClick={printable} className="btn-outline text-[11px] py-1.5"><Printer className="w-3.5 h-3.5" />Print</button>
		</div>
		<input ref={fileInput} type="file" accept="application/json" className="hidden" onChange={(event) => void importFile(event.target.files?.[0])} />
		{error && <p className="text-[10px]" style={{ color: '#b91c1c' }}>{error}</p>}
	</section>;
};
