import { Database, Filter, Layers, Search } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';
import { EthIcon, UsdcIcon, UsdtIcon } from './Icons';

interface HeaderProps {
	currentAddress: string;
	onSearch: (addresses: string[], tokens: string[]) => void;
	network: string;
}

export const Header: React.FC<HeaderProps> = ({ currentAddress, onSearch, network }) => {
	const [isMultiMode, setIsMultiMode] = useState(false);
	const [singleInput, setSingleInput] = useState(currentAddress);
	const [bulkInput, setBulkInput] = useState('');
	const [selectedTokens, setSelectedTokens] = useState<string[]>(['ETH', 'USDT']);

	const toggleToken = (token: string) => {
		if (selectedTokens.includes(token)) {
			setSelectedTokens(selectedTokens.filter((t) => t !== token));
		} else {
			setSelectedTokens([...selectedTokens, token]);
		}
	};

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault();
		if (isMultiMode) {
			const addrs = bulkInput
				.split(/[\s,\n]+/)
				.map((a) => a.trim())
				.filter(Boolean);
			if (addrs.length > 0) onSearch(addrs, selectedTokens);
		} else if (singleInput.trim()) {
			onSearch([singleInput.trim()], selectedTokens);
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
						Fund Flow &amp; Multi-Address Investigation
					</p>
				</div>
			</div>

			{/* Search */}
			<form onSubmit={handleSubmit} className="flex-1 max-w-2xl w-full">
				<div className="flex flex-col gap-1.5">
					{/* Mode + Token row */}
					<div className="flex items-center justify-between px-0.5">
						<div className="flex items-center gap-1.5">
							<button
								type="button"
								onClick={() => setIsMultiMode(false)}
								className="px-2.5 py-0.5 rounded-full text-[11px] font-medium transition"
								style={
									!isMultiMode
										? {
												background: 'linear-gradient(135deg, var(--prism-4), var(--prism-5))',
												color: '#fff',
												border: 'none',
											}
										: {
												background: 'var(--slate)',
												color: 'var(--ink-2)',
												border: '1px solid var(--border)',
											}
								}
							>
								Single
							</button>
							<button
								type="button"
								onClick={() => setIsMultiMode(true)}
								className="px-2.5 py-0.5 rounded-full text-[11px] font-medium transition flex items-center gap-1"
								style={
									isMultiMode
										? {
												background: 'linear-gradient(135deg, var(--prism-4), var(--prism-5))',
												color: '#fff',
												border: 'none',
											}
										: {
												background: 'var(--slate)',
												color: 'var(--ink-2)',
												border: '1px solid var(--border)',
											}
								}
							>
								<Layers className="w-3 h-3" />
								Multi-Address
							</button>
						</div>

						{/* Token filters */}
						<div className="flex items-center gap-1">
							<Filter className="w-3 h-3" style={{ color: 'var(--ink-3)' }} />
							{[
								{ key: 'ETH', Icon: EthIcon, color: '#627EEA' },
								{ key: 'USDT', Icon: UsdtIcon, color: '#26A17B' },
								{ key: 'USDC', Icon: UsdcIcon, color: '#2775CA' },
							].map(({ key, Icon, color }) => {
								const active = selectedTokens.includes(key);
								return (
									<button
										key={key}
										type="button"
										onClick={() => toggleToken(key)}
										className="px-2 py-0.5 rounded-full text-[11px] font-medium flex items-center gap-1 transition"
										style={
											active
												? {
														background: `${color}18`,
														border: `1px solid ${color}55`,
														color: color,
													}
												: {
														background: 'var(--slate)',
														border: '1px solid var(--border)',
														color: 'var(--ink-3)',
													}
										}
									>
										<Icon className="w-3 h-3" />
										{key}
									</button>
								);
							})}
						</div>
					</div>

					{/* Input */}
					{isMultiMode ? (
						<div className="relative">
							<textarea
								rows={2}
								value={bulkInput}
								onChange={(e) => setBulkInput(e.target.value)}
								placeholder="Enter addresses separated by spaces or newlines…"
								className="prism-input font-mono resize-none pr-28"
								style={{ paddingTop: '0.5rem', paddingBottom: '0.5rem' }}
							/>
							<button
								type="submit"
								className="btn-primary absolute right-2 bottom-2 text-[11px]"
								style={{ padding: '0.3rem 0.75rem' }}
							>
								Analyze Flow
							</button>
						</div>
					) : (
						<div className="relative">
							<Search
								className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2"
								style={{ color: 'var(--ink-3)' }}
							/>
							<input
								type="text"
								value={singleInput}
								onChange={(e) => setSingleInput(e.target.value)}
								placeholder="Search target address (0x…)"
								className="prism-input font-mono pl-9 pr-20"
							/>
							<button
								type="submit"
								className="btn-primary absolute right-1.5 top-1/2 -translate-y-1/2 text-[11px]"
								style={{ padding: '0.3rem 0.75rem' }}
							>
								Trace
							</button>
						</div>
					)}
				</div>
			</form>

			{/* Network badge */}
			<div
				className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg shrink-0 text-xs"
				style={{
					background: 'var(--slate)',
					border: '1px solid var(--border)',
					color: 'var(--ink-2)',
				}}
			>
				<Database className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />
				<span className="font-mono">{network}</span>
			</div>
		</header>
	);
};
