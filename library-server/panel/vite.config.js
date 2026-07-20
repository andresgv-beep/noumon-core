import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Panel de Control del paquete Library Server. Se sirve bajo /panel/.
// - dev (npm run dev): sirve en :5174 y proxya /api al Core (:8090).
// - build (npm run build): compila estaticos a ../core/www-panel.
export default defineConfig({
  base: '/panel/',
  plugins: [svelte()],
  server: {
    port: 5174,
    proxy: {
      // changeOrigin:false conserva el Host original (localhost:5174): así el
      // guardián anti-CSRF del Core ve Origin == Host y deja pasar el login en
      // desarrollo sin necesitar DEV_CORS ni tocar el servicio.
      '/api': { target: 'http://127.0.0.1:8090', changeOrigin: false },
      '/content': { target: 'http://127.0.0.1:8090', changeOrigin: false },
    },
  },
  build: {
    outDir: '../core/www-panel',
    emptyOutDir: true,
  },
})
