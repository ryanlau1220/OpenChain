import { HeadContent, Outlet, Scripts, createRootRoute } from '@tanstack/react-router';
import cssUrl from '../styles.css?url';

export const Route = createRootRoute({
	head: () => ({
		meta: [
			{ charSet: 'utf-8' },
			{ name: 'viewport', content: 'width=device-width, initial-scale=1' },
			{ title: 'OpenChain - Blockchain Investigation Platform' },
		],
		links: [
			{ rel: 'icon', href: '/favicon.png', type: 'image/png' },
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
			<body className="bg-slate-950 text-slate-100 min-h-screen antialiased">
				<Outlet />
				<Scripts />
			</body>
		</html>
	);
}
