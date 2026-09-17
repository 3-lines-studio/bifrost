import { defineConfig } from "vite";

export default defineConfig({
  define: {
    __APP_ROUTER_EXAMPLE__: JSON.stringify(true),
  },
});
