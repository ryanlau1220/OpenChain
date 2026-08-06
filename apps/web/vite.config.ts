import tailwindcss from '@tailwindcss/vite';
import { devtools } from '@tanstack/devtools-vite';
import { tanstackStart } from '@tanstack/react-start/plugin/vite';
import viteReact from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [devtools(), tailwindcss(), tanstackStart(), viteReact()],
	server: {
		port: 3000,
		host: true,
	},
	build: {
		rollupOptions: {
			output: {
				manualChunks(id) {
					if (id.includes('node_modules')) {
						if (id.includes('cytoscape')) {
							return 'vendor-cytoscape';
						}
					}
				},
			},
		},
	},
});
