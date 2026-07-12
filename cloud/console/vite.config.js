import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5180,
    host: true,
    // same-origin API in dev: no CORS, cookies just work.
    // changeOrigin must stay OFF: the API's same-origin (CSRF) check compares
    // the browser's Origin header against Host, so the proxied request must
    // keep Host = localhost:5180.
    proxy: {
      "/api": { target: "http://127.0.0.1:8787", changeOrigin: false, rewrite: (p) => p.replace(/^\/api/, "") },
    },
  },
});
