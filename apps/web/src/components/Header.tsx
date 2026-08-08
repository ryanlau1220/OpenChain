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
			if (addrs.length > 0) {
				onSearch(addrs, selectedTokens);
			}
		} else if (singleInput.trim()) {
			onSearch([singleInput.trim()], selectedTokens);
		}
	};

	return (
		<header className="border-b border-slate-800 bg-slate-900 px-6 py-3 flex flex-col md:flex-row items-center justify-between sticky top-0 z-50 gap-4">
			{/* Brand & Logo */}
			<div className="flex items-center gap-3">
				<img
					src="/logo.png"
					alt="OpenChain Logo"
					className="w-9 h-9 rounded-lg object-cover shadow border border-emerald-500/30"
				/>
				<div>
					<div className="flex items-center gap-2">
						<span className="font-bold text-base tracking-wide text-slate-100">OpenChain</span>
						<span className="text-[10px] uppercase font-bold px-2 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20">
							TRACE
						</span>
					</div>
					<p className="text-[11px] text-slate-400">
						Blockchain Fund Flow & Multi-Address Investigation
					</p>
				</div>
			</div>

			{/* Search Input & Token Multiselect Controls */}
			<form onSubmit={handleSubmit} className="flex-1 max-w-2xl w-full">
				<div className="flex flex-col gap-2">
					<div className="flex items-center justify-between text-xs text-slate-400 px-1">
						<div className="flex items-center gap-2">
							<button
								type="button"
								onClick={() => setIsMultiMode(false)}
								className={`px-2.5 py-0.5 rounded font-medium text-[11px] transition ${!isMultiMode ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-slate-200'}`}
							>
								Single Address
							</button>
							<button
								type="button"
								onClick={() => setIsMultiMode(true)}
								className={`px-2.5 py-0.5 rounded font-medium text-[11px] transition flex items-center gap-1.5 ${isMultiMode ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-slate-200'}`}
							>
								<Layers className="w-3 h-3" />
								Multi-Address Investigation
							</button>
						</div>

						{/* Token Multiselect Filters */}
						<div className="flex items-center gap-1.5">
							<Filter className="w-3 h-3 text-slate-500" />
							<button
								type="button"
								onClick={() => toggleToken('ETH')}
								className={`px-2 py-0.5 text-[11px] rounded font-medium flex items-center gap-1 transition ${selectedTokens.includes('ETH') ? 'bg-slate-700 text-slate-100 border border-blue-500/50' : 'bg-slate-800 text-slate-400 hover:bg-slate-750'}`}
							>
								<EthIcon className="w-3 h-3" />
								ETH
							</button>
							<button
								type="button"
								onClick={() => toggleToken('USDT')}
								className={`px-2 py-0.5 text-[11px] rounded font-medium flex items-center gap-1 transition ${selectedTokens.includes('USDT') ? 'bg-slate-700 text-slate-100 border border-emerald-500/50' : 'bg-slate-800 text-slate-400 hover:bg-slate-750'}`}
							>
								<UsdtIcon className="w-3 h-3" />
								USDT
							</button>
							<button
								type="button"
								onClick={() => toggleToken('USDC')}
								className={`px-2 py-0.5 text-[11px] rounded font-medium flex items-center gap-1 transition ${selectedTokens.includes('USDC') ? 'bg-slate-700 text-slate-100 border border-blue-400/50' : 'bg-slate-800 text-slate-400 hover:bg-slate-750'}`}
							>
								<UsdcIcon className="w-3 h-3" />
								USDC
							</button>
						</div>
					</div>

					{isMultiMode ? (
						<div className="relative">
							<textarea
								rows={2}
								value={bulkInput}
								onChange={(e) => setBulkInput(e.target.value)}
								placeholder="Please enter address (separated by spaces or newlines)"
								className="w-full bg-slate-950 border border-slate-700 rounded-lg p-2.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500 transition font-mono resize-none"
							/>
							<button
								type="submit"
								className="absolute right-2 bottom-2.5 px-3 py-1 bg-blue-600 hover:bg-blue-500 text-white font-medium text-xs rounded transition shadow"
							>
								Analyze Flow
							</button>
						</div>
					) : (
						<div className="relative">
							<Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
							<input
								type="text"
								value={singleInput}
								onChange={(e) => setSingleInput(e.target.value)}
								placeholder="Search target address (0x...)"
								className="w-full bg-slate-950 border border-slate-700 rounded-lg pl-9 pr-20 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500 transition font-mono"
							/>
							<button
								type="submit"
								className="absolute right-1.5 top-1/2 -translate-y-1/2 px-3 py-1 bg-blue-600 hover:bg-blue-500 text-white font-medium text-xs rounded transition shadow"
							>
								Trace
							</button>
						</div>
					)}
				</div>
			</form>

			{/* Network Indicator */}
			<div className="flex items-center gap-2">
				<div className="flex items-center gap-2 px-3 py-1.5 rounded bg-slate-950 border border-slate-800 text-xs">
					<Database className="w-3.5 h-3.5 text-blue-400" />
					<span className="text-slate-300 font-mono">{network}</span>
				</div>
			</div>
		</header>
	);
};
