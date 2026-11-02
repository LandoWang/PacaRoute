import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    proxy: {
      "/api": {
        target: process.env.PACAROUTE_API || "http://127.0.0.1:8766",
        changeOrigin: true,
        configure(proxy) {
          proxy.on("proxyReq", (request) => {
            request.setHeader(
              "Origin",
              process.env.PACAROUTE_API || "http://127.0.0.1:8766",
            );
          });
        },
      },
    },
  },
  build: { outDir: "dist", emptyOutDir: true, assetsInlineLimit: 0 },
});
