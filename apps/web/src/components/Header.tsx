import { Search } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';
import {
	type SupportedNetwork,
	detectAddressNetwork,
	isEVMAddress,
	networkDetails,
	supportedNetworks,
} from '../services/api';

interface HeaderProps {
	currentAddress: string;
	onSearch: (address: string, detectedNetwork?: SupportedNetwork) => void;
	network: SupportedNetwork;
	onNetworkChange: (network: SupportedNetwork) => void;
}

export const Header: React.FC<HeaderProps> = ({
	currentAddress,
	onSearch,
	network,
	onNetworkChange,
}) => {
	const [singleInput, setSingleInput] = useState(currentAddress);
	const detectedNetwork = detectAddressNetwork(singleInput);
	const detectedDetails =
		detectedNetwork === undefined ? undefined : networkDetails(detectedNetwork);
	const hasEVMAddress = isEVMAddress(singleInput);

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault();
		if (singleInput.trim()) {
			onSearch(singleInput.trim(), detectedNetwork);
		}
	};

	const handleInputChange = (value: string) => {
		setSingleInput(value);
		const detected = detectAddressNetwork(value);
		if (detected !== undefined && detected !== network) onNetworkChange(detected);
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
							<img
								src={networkDetails(network).icon}
								alt={`${networkDetails(network).name} icon`}
								className="h-3.5 w-3.5"
							/>
							<select
								aria-label="Network"
								value={network}
								onChange={(event) =>
									onNetworkChange(Number(event.target.value) as SupportedNetwork)
								}
								className="bg-transparent outline-none cursor-pointer"
							>
								<optgroup label="EVM Networks">
									{supportedNetworks.slice(0, 2).map((item) => (
										<option key={item} value={item}>
											{networkDetails(item).name}
										</option>
									))}
								</optgroup>
								<optgroup label="Other Networks">
									{supportedNetworks.slice(2).map((item) => (
										<option key={item} value={item}>
											{networkDetails(item).name}
										</option>
									))}
								</optgroup>
							</select>
						</div>
						{hasEVMAddress && (
							<span className="text-[10px]" style={{ color: 'var(--accent)' }}>
								EVM address — choose its network
							</span>
						)}
						{detectedDetails && detectedNetwork !== network && (
							<span className="text-[10px]" style={{ color: 'var(--accent)' }}>
								<img
									src={detectedDetails.icon}
									alt={`${detectedDetails.name} icon`}
									className="mr-1 inline h-3.5 w-3.5 align-text-bottom"
								/>
								{detectedDetails.name} detected
							</span>
						)}
					</div>

					{/* Single Input */}
					<div className="relative">
						<input
							type="text"
							value={singleInput}
							onChange={(e) => handleInputChange(e.target.value)}
							placeholder="Search target address"
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
