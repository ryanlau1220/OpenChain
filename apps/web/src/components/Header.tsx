import { Coins, Database, Layers, Radio, Search, Shield } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';

interface HeaderProps {
	currentAddress: string;
	onSearch: (addresses: string[], tokens: string[]) => void;
	network: string;
}

export const Header: React.FC<HeaderProps> = ({
	currentAddress,
	onSearch,
	network,
}) => {
	const [isMultiMode, setIsMultiMode] = useState(false);
	const [singleInput, setSingleInput] = useState(currentAddress);
	const [bulkInput, setBulkInput] = useState('');
	const [selectedTokens, setSelectedTokens] = useState<string[]>([
		'ETH',
		'USDT',
	]);

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
			if (addrs.length > 0) {
				onSearch(addrs, selectedTokens);
			}
		} else if (singleInput.trim()) {
			onSearch([singleInput.trim()], selectedTokens);
		}
	};

	return (
		<header className="border-b border-slate-800/80 glass-panel px-6 py-3 flex flex-col md:flex-row items-center justify-between sticky top-0 z-50 gap-4">
			{/* Brand & Logo */}
			<div className="flex items-center gap-3">
				<div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-cyan-500 to-blue-600 flex items-center justify-center shadow-lg shadow-cyan-500/20">
					<Shield className="w-5 h-5 text-white" />
				</div>
				<div>
					<div className="flex items-center gap-2">
						<span className="font-bold text-lg tracking-wide text-white font-sans">
							OpenChain
						</span>
						<span className="text-[10px] uppercase font-semibold px-2 py-0.5 rounded-full bg-cyan-500/10 text-cyan-400 border border-cyan-500/20">
							TRACE
						</span>
					</div>
					<p className="text-xs text-slate-400">
						Beosin-Style Fund Flow Investigation
					</p>
				</div>
			</div>

			{/* Multi-Address & Token Search Bar */}
			<form onSubmit={handleSubmit} className="flex-1 max-w-2xl w-full">
				<div className="flex flex-col gap-2">
					<div className="flex items-center justify-between text-xs text-slate-400 px-1">
						<div className="flex items-center gap-3">
							<button
								type="button"
								onClick={() => setIsMultiMode(false)}
								className={`px-2.5 py-0.5 rounded-md font-medium transition ${!isMultiMode ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30' : 'text-slate-400 hover:text-slate-200'}`}
							>
								Single Address
							</button>
							<button
								type="button"
								onClick={() => setIsMultiMode(true)}
								className={`px-2.5 py-0.5 rounded-md font-medium transition flex items-center gap-1 ${isMultiMode ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30' : 'text-slate-400 hover:text-slate-200'}`}
							>
								<Layers className="w-3 h-3" />
								Multi-Address Investigation
							</button>
						</div>

						{/* Token Multiselect Filters */}
						<div className="flex items-center gap-1.5">
							<Coins className="w-3.5 h-3.5 text-slate-400" />
							{['ETH', 'USDT', 'USDC'].map((token) => {
								const active = selectedTokens.includes(token);
								return (
									<button
										key={token}
										type="button"
										onClick={() => toggleToken(token)}
										className={`px-2 py-0.5 text-[11px] rounded font-mono transition ${active ? 'bg-blue-600 text-white font-bold' : 'bg-slate-800 text-slate-400 hover:bg-slate-700'}`}
									>
										{token}
									</button>
								);
							})}
						</div>
					</div>

					{isMultiMode ? (
						<div className="relative">
							<textarea
								rows={2}
								value={bulkInput}
								onChange={(e) => setBulkInput(e.target.value)}
								placeholder="Please enter address (separated by spaces or newlines)..."
								className="w-full bg-slate-900/90 border border-cyan-500/40 rounded-xl p-3 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-cyan-400 focus:ring-1 focus:ring-cyan-400 transition font-mono resize-none"
							/>
							<button
								type="submit"
								className="absolute right-2 bottom-2.5 px-3 py-1 bg-cyan-500 hover:bg-cyan-400 text-slate-950 font-bold text-xs rounded-lg transition shadow-md"
							>
								Analyze Flow
							</button>
						</div>
					) : (
						<div className="relative">
							<Search className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-400" />
							<input
								type="text"
								value={singleInput}
								onChange={(e) => setSingleInput(e.target.value)}
								placeholder="Search target address (0x...)"
								className="w-full bg-slate-900/90 border border-slate-700/60 rounded-xl pl-10 pr-24 py-2 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-cyan-500 focus:ring-1 focus:ring-cyan-500 transition font-mono"
							/>
							<button
								type="submit"
								className="absolute right-2 top-1/2 -translate-y-1/2 px-3 py-1 bg-cyan-500 hover:bg-cyan-400 text-slate-950 font-bold text-xs rounded-lg transition shadow-md"
							>
								Trace
							</button>
						</div>
					)}
				</div>
			</form>

			{/* Network & Live Status */}
			<div className="flex items-center gap-3">
				<div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-900/80 border border-slate-800 text-xs">
					<Database className="w-3.5 h-3.5 text-cyan-400" />
					<span className="text-slate-300 font-mono">{network}</span>
				</div>

				<div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-xs text-emerald-400">
					<Radio className="w-3.5 h-3.5 animate-pulse" />
					<span className="font-medium">Live EVM</span>
				</div>
			</div>
		</header>
	);
};
