import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [react()],
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
						if (id.includes('react') || id.includes('react-dom')) {
							return 'vendor-react';
						}
						if (id.includes('lucide-react')) {
							return 'vendor-icons';
						}
						return 'vendor';
					}
				},
			},
		},
	},
});
