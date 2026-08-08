import { Check, Copy, Share2, X } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';

interface ShareModalProps {
	isOpen: boolean;
	onClose: () => void;
	shareUrl: string;
	expiresAt?: string;
}

export const ShareModal: React.FC<ShareModalProps> = ({ isOpen, onClose, shareUrl, expiresAt }) => {
	const [copied, setCopied] = useState(false);

	if (!isOpen) return null;

	const handleCopy = () => {
		navigator.clipboard.writeText(shareUrl);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<div
			className="fixed inset-0 z-50 flex items-center justify-center p-4"
			style={{ background: 'rgba(26,29,35,0.35)', backdropFilter: 'blur(8px)' }}
		>
			<div
				className="max-w-lg w-full p-6 rounded-2xl relative rise-in"
				style={{
					background: 'var(--white)',
					border: '1px solid var(--border)',
					boxShadow: '0 24px 48px rgba(26,29,35,0.14), 0 4px 12px rgba(136,125,255,0.12)',
				}}
			>
				{/* Close */}
				<button
					type="button"
					onClick={onClose}
					className="absolute right-4 top-4 p-1.5 rounded-lg transition hover:bg-[var(--slate)]"
					style={{ color: 'var(--ink-3)' }}
				>
					<X className="w-4 h-4" />
				</button>

				{/* Header */}
				<div className="flex items-center gap-3 mb-5">
					<div
						className="w-10 h-10 rounded-xl flex items-center justify-center"
						style={{
							background: 'linear-gradient(135deg, rgba(136,125,255,0.12), rgba(167,249,255,0.12))',
							border: '1px solid rgba(136,125,255,0.25)',
						}}
					>
						<Share2 className="w-5 h-5" style={{ color: 'var(--accent)' }} />
					</div>
					<div>
						<h3 className="text-sm font-bold" style={{ color: 'var(--ink)' }}>
							Share Canvas
						</h3>
						<p className="text-xs mt-0.5" style={{ color: 'var(--ink-3)' }}>
							Share this graph view with your team
						</p>
					</div>
				</div>

				{/* URL block */}
				<div
					className="p-3 rounded-xl mb-4 space-y-2"
					style={{ background: 'var(--snow)', border: '1px solid var(--border)' }}
				>
					<p
						className="text-[10px] font-medium uppercase tracking-widest"
						style={{ color: 'var(--ink-3)' }}
					>
						Share URL
					</p>
					<div
						className="break-all text-[11px] font-mono p-2.5 rounded-lg select-all"
						style={{
							background: 'var(--white)',
							border: '1px solid var(--border)',
							color: 'var(--accent)',
						}}
					>
						{shareUrl}
					</div>
					{expiresAt && (
						<p className="text-[10px]" style={{ color: 'var(--ink-3)' }}>
							Valid until {new Date(expiresAt).toLocaleString()}
						</p>
					)}
				</div>

				{/* Actions */}
				<div className="flex items-center justify-end gap-2">
					<button type="button" onClick={onClose} className="btn-outline text-xs">
						Cancel
					</button>
					<button
						type="button"
						onClick={handleCopy}
						className="btn-primary text-xs flex items-center gap-1.5"
					>
						{copied ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
						{copied ? 'Copied!' : 'Copy Link'}
					</button>
				</div>
			</div>
		</div>
	);
};
