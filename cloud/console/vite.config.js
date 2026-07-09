import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5180,
    host: true,
    // same-origin API in dev: no CORS, cookies just work
    proxy: {
      "/api": { target: "http://127.0.0.1:8787", changeOrigin: true, rewrite: (p) => p.replace(/^\/api/, "") },
    },
  },
});
