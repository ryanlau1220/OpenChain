import { ChevronDown, LoaderCircle, Search } from 'lucide-react';
import type React from 'react';
import { useEffect, useRef, useState } from 'react';
import {
	Network,
	type SupportedNetwork,
	detectAddressNetwork,
	isEVMNetwork,
	networkDetails,
	supportedNetworks,
} from '../services/api';

interface HeaderProps {
	currentAddress: string;
	onSearch: (address: string, detectedNetwork?: SupportedNetwork) => void;
	network: SupportedNetwork;
	onNetworkChange: (network: SupportedNetwork) => void;
	loading?: boolean;
}

export const Header: React.FC<HeaderProps> = ({
	currentAddress,
	onSearch,
	network,
	onNetworkChange,
	loading = false,
}) => {
	const [singleInput, setSingleInput] = useState(currentAddress);
	const [networkMenuOpen, setNetworkMenuOpen] = useState(false);
	const [networkQuery, setNetworkQuery] = useState('');
	const networkMenuRef = useRef<HTMLDivElement>(null);
	const networkTriggerRef = useRef<HTMLButtonElement>(null);

	useEffect(() => setSingleInput(currentAddress), [currentAddress]);

	useEffect(() => {
		if (!networkMenuOpen) return;
		const closeOutsideMenu = (event: PointerEvent) => {
			if (!networkMenuRef.current?.contains(event.target as Node)) setNetworkMenuOpen(false);
		};
		document.addEventListener('pointerdown', closeOutsideMenu);
		return () => document.removeEventListener('pointerdown', closeOutsideMenu);
	}, [networkMenuOpen]);
	useEffect(() => {
		if (!networkMenuOpen) return;
		const closeOnEscape = (event: KeyboardEvent) => {
			if (event.key !== 'Escape') return;
			setNetworkMenuOpen(false);
			networkTriggerRef.current?.focus();
		};
		document.addEventListener('keydown', closeOnEscape);
		return () => document.removeEventListener('keydown', closeOnEscape);
	}, [networkMenuOpen]);
	const inputNetwork = (value: string) => {
		const detected = detectAddressNetwork(value);
		// Keep an explicit EVM network choice: an address alone cannot distinguish
		// Ethereum from Base. Otherwise, a 0x address switches non-EVM views to Ethereum.
		if (detected === Network.ETHEREUM_MAINNET && isEVMNetwork(network)) return network;
		return detected;
	};

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault();
		if (singleInput.trim()) {
			onSearch(singleInput.trim(), inputNetwork(singleInput));
		}
	};

	const handleInputChange = (value: string) => {
		setSingleInput(value);
		const detected = inputNetwork(value);
		if (detected !== undefined && detected !== network) onNetworkChange(detected);
	};
	const selectNetwork = (nextNetwork: SupportedNetwork) => {
		setNetworkMenuOpen(false);
		setNetworkQuery('');
		onNetworkChange(nextNetwork);
	};
	const networkGroups = [
		{ label: 'EVM networks', items: supportedNetworks.filter(isEVMNetwork) },
		{ label: 'Other networks', items: supportedNetworks.filter((item) => !isEVMNetwork(item)) },
	] as const;
	const normalizedNetworkQuery = networkQuery.trim().toLowerCase();
	const visibleGroups = networkGroups
		.map((group) => ({
			...group,
			items: group.items.filter((item) =>
				networkDetails(item).name.toLowerCase().includes(normalizedNetworkQuery),
			),
		}))
		.filter((group) => group.items.length > 0);
	const selectedNetwork = networkDetails(network);

	return (
		<header
			style={{
				background: 'rgba(255,255,255,0.85)',
				borderBottom: '1px solid var(--border)',
				backdropFilter: 'blur(12px)',
				WebkitBackdropFilter: 'blur(12px)',
			}}
			className="px-5 py-3 flex flex-col sm:flex-row items-center justify-between sticky top-0 z-50 gap-3"
		>
			<a href="#investigation-workspace" className="sr-only focus:not-sr-only">
				Skip to investigation workspace
			</a>
			<div className="flex items-center gap-3 shrink-0">
				<img
					src="/logo.png"
					alt="OpenChain Logo"
					className="w-8 h-8 rounded-lg object-cover"
					style={{ boxShadow: '0 0 0 1px var(--border)' }}
				/>
				<span className="font-bold text-sm tracking-tight" style={{ color: 'var(--ink)' }}>
					OpenChain
				</span>
			</div>

			<form onSubmit={handleSubmit} className="w-full max-w-2xl">
				<div
					className="flex h-11 rounded-xl"
					style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
				>
					<div ref={networkMenuRef} className="relative shrink-0">
						<button
							ref={networkTriggerRef}
							type="button"
							aria-label="Network"
							aria-expanded={networkMenuOpen}
							onClick={() => {
								setNetworkQuery('');
								setNetworkMenuOpen((open) => !open);
							}}
							className="flex h-full items-center gap-2 border-r px-3 text-xs font-medium transition hover:bg-[var(--snow)]"
							style={{ borderColor: 'var(--border)', color: 'var(--ink-2)' }}
						>
							<img
								src={selectedNetwork.icon}
								alt={`${selectedNetwork.name} icon`}
								className="h-4 w-4"
							/>
							<span className="hidden md:inline">{selectedNetwork.name}</span>
							<ChevronDown className="h-3.5 w-3.5" />
						</button>
						{networkMenuOpen && (
							<div
								aria-label="Network choices"
								className="absolute left-0 top-[calc(100%+0.4rem)] z-50 w-56 rounded-xl p-1.5 shadow-lg"
								style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
							>
								<input
									aria-label="Find network"
									value={networkQuery}
									onChange={(event) => setNetworkQuery(event.target.value)}
									placeholder="Find network"
									className="prism-input m-1 px-2 py-1.5 text-xs"
									style={{ width: 'calc(100% - 0.5rem)' }}
								/>
								{visibleGroups.map((group) => (
									<div key={group.label} className="py-1">
										<p
											className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider"
											style={{ color: 'var(--ink-3)' }}
										>
											{group.label}
										</p>
										{group.items.map((item) => {
											const details = networkDetails(item);
											return (
												<button
													key={item}
													type="button"
													aria-pressed={item === network}
													onClick={() => selectNetwork(item)}
													className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs hover:bg-[var(--slate)]"
													style={{ color: item === network ? 'var(--accent)' : 'var(--ink-2)' }}
												>
													<img src={details.icon} alt="" className="h-4 w-4" />
													{details.name}
												</button>
											);
										})}
									</div>
								))}
								{visibleGroups.length === 0 && (
									<p className="px-2 py-3 text-xs" style={{ color: 'var(--ink-3)' }}>
										No network found
									</p>
								)}
							</div>
						)}
					</div>
					<input
						type="text"
						name="address"
						autoComplete="off"
						spellCheck={false}
						aria-label="Search target address"
						value={singleInput}
						onChange={(event) => handleInputChange(event.target.value)}
						placeholder="Search target address"
						className="min-w-0 flex-1 bg-transparent px-3 font-mono text-sm"
					/>
					<button
						type="submit"
						disabled={loading}
						className="btn-primary m-1 px-3"
						title="Investigate Address"
						aria-label="Investigate address"
					>
						{loading ? (
							<LoaderCircle className="h-4 w-4 animate-spin" />
						) : (
							<Search className="h-4 w-4" />
						)}
					</button>
				</div>
			</form>
		</header>
	);
};
