import { Search } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';
import { EthIcon } from './Icons';

interface HeaderProps {
	currentAddress: string;
	onSearch: (address: string) => void;
	network: string;
}

export const Header: React.FC<HeaderProps> = ({ currentAddress, onSearch, network }) => {
	const [singleInput, setSingleInput] = useState(currentAddress);

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault();
		if (singleInput.trim()) {
			onSearch(singleInput.trim());
		}
	};

	return (
		<header
			style={{
				background: 'rgba(255,255,255,0.85)',
				borderBottom: '1px solid var(--border)',
				backdropFilter: 'blur(12px)',
				WebkitBackdropFilter: 'blur(12px)',
			}}
			className="px-5 py-2.5 flex flex-col md:flex-row items-center justify-between sticky top-0 z-50 gap-3"
		>
			{/* Brand */}
			<div className="flex items-center gap-3 shrink-0">
				<img
					src="/logo.png"
					alt="OpenChain Logo"
					className="w-8 h-8 rounded-lg object-cover"
					style={{ boxShadow: '0 0 0 1px var(--border)' }}
				/>
				<div>
					<div className="flex items-center gap-2">
						<span className="font-bold text-sm tracking-tight" style={{ color: 'var(--ink)' }}>
							OpenChain
						</span>
						<span
							className="text-[9px] uppercase font-bold px-1.5 py-0.5 rounded-full"
							style={{
								background:
									'linear-gradient(135deg, rgba(136,125,255,0.12), rgba(167,249,255,0.12))',
								border: '1px solid rgba(136,125,255,0.28)',
								color: 'var(--accent)',
							}}
						>
							TRACE
						</span>
					</div>
					<p className="text-[10px] mt-0.5" style={{ color: 'var(--ink-3)' }}>
						Fund Flow Investigation
					</p>
				</div>
			</div>

			{/* Search */}
			<form onSubmit={handleSubmit} className="flex-1 max-w-2xl w-full">
				<div className="flex flex-col gap-1.5">
					{/* Network Badge Row */}
					<div className="flex items-center justify-end px-0.5">
						<div
							className="flex items-center gap-1.5 px-3 py-1 rounded-full text-[11px] font-medium"
							style={{
								background: 'rgba(98, 126, 234, 0.10)',
								border: '1px solid rgba(98, 126, 234, 0.30)',
								color: '#627EEA',
							}}
						>
							<EthIcon className="w-3.5 h-3.5" />
							<span>{network}</span>
						</div>
					</div>

					{/* Single Input */}
					<div className="relative">
						<input
							type="text"
							value={singleInput}
							onChange={(e) => setSingleInput(e.target.value)}
							placeholder="Search target address (0x…)"
							className="prism-input font-mono pl-3.5 pr-12"
						/>
						<button
							type="submit"
							className="btn-primary absolute right-1.5 top-1/2 -translate-y-1/2 text-xs flex items-center justify-center p-2 rounded-lg"
							title="Investigate Address"
						>
							<Search className="w-4 h-4" />
						</button>
					</div>
				</div>
			</form>
		</header>
	);
};
