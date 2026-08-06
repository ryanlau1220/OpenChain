import { createRouter as createTanStackRouter } from '@tanstack/react-router';
import { routeTree } from './routeTree.gen';

function NotFound() {
	return (
		<div className="min-h-screen bg-slate-950 flex flex-col items-center justify-center text-slate-100 p-6 text-center">
			<h1 className="text-4xl font-bold text-blue-400 font-mono mb-2">404</h1>
			<p className="text-slate-400 text-sm mb-4">
				The requested investigation route does not exist.
			</p>
			<a
				href="/"
				className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded text-xs font-medium transition"
			>
				Return to Investigation Canvas
			</a>
		</div>
	);
}

export function getRouter() {
	const router = createTanStackRouter({
		routeTree,
		scrollRestoration: true,
		defaultPreload: 'intent',
		defaultPreloadStaleTime: 0,
		defaultNotFoundComponent: NotFound,
	});

	return router;
}

declare module '@tanstack/react-router' {
	interface Register {
		router: ReturnType<typeof getRouter>;
	}
}
