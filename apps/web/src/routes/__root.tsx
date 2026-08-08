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
