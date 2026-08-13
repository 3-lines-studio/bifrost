import { fileURLToPath, URL } from "node:url";
import babel from "@rolldown/plugin-babel";
import { defineConfig, type Plugin } from "vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

function exampleVirtualModule(): Plugin {
  const id = "virtual:bifrost-example";
  const resolved = `\0${id}`;
  return {
    name: "bifrost-example-virtual-module",
    resolveId(source) {
      return source === id ? resolved : null;
    },
    load(source) {
      return source === resolved ? `export const source = "vite-plugin";` : null;
    },
  };
}

export default defineConfig({
  resolve: {
    alias: {
      "@example": fileURLToPath(new URL("./example/basic/pages", import.meta.url)),
    },
  },
  plugins: [
    react(),
    babel({ presets: [reactCompilerPreset()] }),
    tailwindcss(),
    exampleVirtualModule(),
  ],
});
