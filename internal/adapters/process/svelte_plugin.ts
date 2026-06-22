function createSveltePlugin(generate) {
  const isClient = generate === "client";
  const cssCacheDir = nodePath.join(
    typeof Bun.env.TMPDIR === "string" && Bun.env.TMPDIR ? Bun.env.TMPDIR : "/tmp",
    "bifrost-svelte-css",
  );
  nodeFs.mkdirSync(cssCacheDir, { recursive: true });

  return {
    name: "svelte-plugin",
    setup(builder) {
      const tsTranspiler = new Bun.Transpiler({ loader: "ts" });

      builder.onLoad({ filter: /\.svelte(\.[jt]s)?$/ }, async (args) => {
        const { compile, compileModule } = await import("svelte/compiler");
        const input = await Bun.file(args.path).text();
        const isModule = /\.svelte\.[jt]s$/.test(args.path);

        if (isModule) {
          const moduleInput = args.path.endsWith(".svelte.ts")
            ? tsTranspiler.transformSync(input)
            : input;
          let modResult;
          try {
            modResult = compileModule(moduleInput, { generate, filename: args.path });
          } catch (e) {
            const msg = e.code ? `${e.message} (${e.code})` : e.message;
            const frame = e.frame ? `\n\n${e.frame}` : "";
            throw new Error(msg + frame);
          }
          return { contents: modResult.js.code, loader: "js" };
        }

        let result;
        try {
          result = compile(input, { generate, filename: args.path });
        } catch (e) {
          const msg = e.code ? `${e.message} (${e.code})` : e.message;
          const frame = e.frame ? `\n\n${e.frame}` : "";
          throw new Error(msg + frame);
        }

        let jsCode = result.js.code;

        if (isClient) {
          const cssCode = result.css?.code?.trim();
          if (cssCode) {
            const hash = Bun.hash(args.path).toString(36);
            const cssFile = nodePath.join(cssCacheDir, `svelte-${hash}.css`);
            nodeFs.writeFileSync(cssFile, cssCode);
            const relPath = nodePath.relative(nodePath.dirname(args.path), cssFile);
            jsCode = `import "${relPath}";\n` + jsCode;
          }
        }

        return { contents: jsCode, loader: "js" };
      });
    },
  };
}
