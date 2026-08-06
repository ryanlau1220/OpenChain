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
			<div className="bg-slate-900 border border-slate-800 rounded-xl max-w-lg w-full p-6 shadow-2xl relative">
				<button
					type="button"
					onClick={onClose}
					className="absolute right-4 top-4 text-slate-400 hover:text-white transition"
				>
					<X className="w-5 h-5" />
				</button>

				<div className="flex items-center gap-3 mb-4">
					<div className="w-9 h-9 rounded-lg bg-blue-600/10 border border-blue-500/20 flex items-center justify-center text-blue-400">
						<Share2 className="w-5 h-5" />
					</div>
					<div>
						<h3 className="text-base font-bold text-slate-100">
							Canvas Sharing
						</h3>
						<p className="text-xs text-slate-400">
							Share the contents of the current canvas with other team users
						</p>
					</div>
				</div>

				<div className="bg-slate-950 border border-slate-800 rounded-lg p-4 mb-4 space-y-2">
					<p className="text-xs text-slate-400 font-mono">Visit the link:</p>
					<div className="break-all text-xs text-blue-400 font-mono bg-slate-900 p-2.5 rounded border border-slate-800 select-all">
						{shareUrl}
					</div>
					{expiresAt && (
						<p className="text-[11px] text-slate-500">
							Viewable shared content (current share is valid until{' '}
							{new Date(expiresAt).toLocaleString()})
						</p>
					)}
				</div>

				<div className="flex items-center justify-end gap-3">
					<button
						type="button"
						onClick={onClose}
						className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 font-medium text-xs rounded-lg transition"
					>
						Cancel
					</button>
					<button
						type="button"
						onClick={handleCopy}
						className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white font-medium text-xs rounded-lg transition shadow flex items-center gap-1.5"
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
