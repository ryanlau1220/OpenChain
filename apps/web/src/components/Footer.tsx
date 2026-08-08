export default function Footer() {
	const year = new Date().getFullYear();
	return (
		<footer
			className="py-4 px-6 flex items-center justify-between text-xs shrink-0"
			style={{
				borderTop: '1px solid var(--border)',
				background: 'rgba(255,255,255,0.70)',
				color: 'var(--ink-3)',
			}}
		>
			<span>© {year} OpenChain. Blockchain Investigation Platform.</span>
			<span
				className="text-[10px] uppercase tracking-widest font-semibold"
				style={{ color: 'var(--muted)' }}
			>
				Sepolia Testnet
			</span>
		</footer>
	);
}
