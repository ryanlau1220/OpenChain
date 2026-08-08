import { HeadContent, Outlet, Scripts, createRootRoute } from '@tanstack/react-router';
import cssUrl from '../styles.css?url';

export const Route = createRootRoute({
	head: () => ({
		meta: [
			{ charSet: 'utf-8' },
			{ name: 'viewport', content: 'width=device-width, initial-scale=1' },
			{ title: 'OpenChain — Blockchain Investigation Platform' },
			{
				name: 'description',
				content:
					'Trace fund flows, map transaction graphs, and investigate blockchain addresses with OpenChain.',
			},
		],
		links: [
			{ rel: 'icon', href: '/favicon.png', type: 'image/png' },
			{ rel: 'preconnect', href: 'https://fonts.googleapis.com' },
			{ rel: 'preconnect', href: 'https://fonts.gstatic.com', crossOrigin: 'anonymous' },
			{ rel: 'stylesheet', href: cssUrl },
		],
	}),
	component: RootComponent,
	errorComponent: RootErrorComponent,
	notFoundComponent: RootNotFoundComponent,
});

function RootComponent() {
	return (
		<html lang="en">
			<head>
				<HeadContent />
			</head>
			<body className="min-h-screen antialiased">
				<Outlet />
				<Scripts />
			</body>
		</html>
	);
}

function RootErrorComponent({ error }: { error: Error }) {
	return (
		<div className="min-h-screen flex items-center justify-center p-6 bg-[var(--snow)]">
			<div
				className="max-w-md w-full p-6 rounded-2xl space-y-4 text-center"
				style={{
					background: 'var(--white)',
					border: '1px solid var(--border)',
					boxShadow: '0 10px 30px rgba(0,0,0,0.05)',
				}}
			>
				<div
					className="w-12 h-12 rounded-full mx-auto flex items-center justify-center font-bold text-red-600"
					style={{ background: 'rgba(239, 68, 68, 0.12)' }}
				>
					!
				</div>
				<h2 className="text-base font-semibold" style={{ color: 'var(--ink)' }}>
					Application Error
				</h2>
				<p
					className="text-xs font-mono p-3 rounded-lg text-left overflow-x-auto"
					style={{ background: 'var(--slate)', color: 'var(--ink-2)' }}
				>
					{error?.message || 'An unexpected error occurred in OpenChain UI.'}
				</p>
				<button
					type="button"
					onClick={() => window.location.reload()}
					className="btn-primary text-xs px-4 py-2 w-full font-medium"
				>
					Reload Application
				</button>
			</div>
		</div>
	);
}

function RootNotFoundComponent() {
	return (
		<div className="min-h-screen flex items-center justify-center p-6 bg-[var(--snow)]">
			<div
				className="max-w-md w-full p-6 rounded-2xl space-y-4 text-center"
				style={{
					background: 'var(--white)',
					border: '1px solid var(--border)',
					boxShadow: '0 10px 30px rgba(0,0,0,0.05)',
				}}
			>
				<h2 className="text-base font-semibold" style={{ color: 'var(--ink)' }}>
					404 — Page Not Found
				</h2>
				<p className="text-xs" style={{ color: 'var(--ink-3)' }}>
					The requested investigation route could not be found.
				</p>
				<a href="/" className="btn-primary text-xs px-4 py-2 inline-block font-medium">
					Return to Investigation App
				</a>
			</div>
		</div>
	);
}
