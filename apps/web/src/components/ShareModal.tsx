import { Check, Copy, Share2, X } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';

interface ShareModalProps {
	isOpen: boolean;
	onClose: () => void;
	shareUrl: string;
	expiresAt?: string;
}

export const ShareModal: React.FC<ShareModalProps> = ({
	isOpen,
	onClose,
	shareUrl,
	expiresAt,
}) => {
	const [copied, setCopied] = useState(false);

	if (!isOpen) return null;

	const handleCopy = () => {
		navigator.clipboard.writeText(shareUrl);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<div className="fixed inset-0 z-50 bg-slate-950/80 backdrop-blur-sm flex items-center justify-center p-4">
			<div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl relative">
				<button
					type="button"
					onClick={onClose}
					className="absolute right-4 top-4 text-slate-400 hover:text-white transition"
				>
					<X className="w-5 h-5" />
				</button>

				<div className="flex items-center gap-3 mb-4">
					<div className="w-10 h-10 rounded-xl bg-cyan-500/10 border border-cyan-500/20 flex items-center justify-center text-cyan-400">
						<Share2 className="w-5 h-5" />
					</div>
					<div>
						<h3 className="text-lg font-bold text-white">Canvas Sharing</h3>
						<p className="text-xs text-slate-400">
							Share the contents of the current canvas with team investigators
						</p>
					</div>
				</div>

				<div className="bg-slate-950 border border-slate-800 rounded-xl p-4 mb-4">
					<p className="text-xs text-slate-400 mb-2 font-mono">
						Visit the link:
					</p>
					<div className="break-all text-xs text-cyan-400 font-mono bg-slate-900 p-2.5 rounded-lg border border-slate-800 select-all">
						{shareUrl}
					</div>
					{expiresAt && (
						<p className="text-[11px] text-slate-500 mt-2">
							Viewable shared content (current share is valid for 7 days until{' '}
							{new Date(expiresAt).toLocaleString()})
						</p>
					)}
				</div>

				<div className="flex items-center justify-end gap-3">
					<button
						type="button"
						onClick={onClose}
						className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 font-medium text-xs rounded-xl transition"
					>
						Cancel
					</button>
					<button
						type="button"
						onClick={handleCopy}
						className="px-4 py-2 bg-cyan-500 hover:bg-cyan-400 text-slate-950 font-bold text-xs rounded-xl transition shadow flex items-center gap-1.5"
					>
						{copied ? (
							<Check className="w-4 h-4" />
						) : (
							<Copy className="w-4 h-4" />
						)}
						{copied ? 'Copied!' : 'Copy Link'}
					</button>
				</div>
			</div>
		</div>
	);
};
