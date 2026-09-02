import { defineConfig } from "vite";

export default defineConfig({
  define: {
    __CONVENTION_EXAMPLE__: JSON.stringify(true),
  },
});
