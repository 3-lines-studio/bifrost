function createSveltePlugin(generate) {
  const isClient = generate === "client";
  const cssCacheDir = nodePath.join(
    typeof Bun.env.TMPDIR === "string" && Bun.env.TMPDIR ? Bun.env.TMPDIR : "/tmp",
    "bifrost-svelte-css",
  );
  nodeFs.mkdirSync(cssCacheDir, { recursive: true });

  function hasTsScript(source) {
    return /<script\s[^>]*\blang="ts"[^>]*>/i.test(source);
  }

  function findTagClose(source, tagStart) {
    let inDq = false;
    let inSq = false;
    for (let i = tagStart + 1; i < source.length; i++) {
      const c = source[i];
      if (c === '"' && !inSq) inDq = !inDq;
      else if (c === "'" && !inDq) inSq = !inSq;
      else if (c === ">" && !inDq && !inSq) return i;
    }
    return -1;
  }

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

        let source = input;
        if (hasTsScript(input)) {
          let out = "";
          let idx = 0;
          while (idx < source.length) {
            const tagStart = source.indexOf("<script", idx);
            if (tagStart === -1) { out += source.slice(idx); break; }
            out += source.slice(idx, tagStart);
            const tagEnd = findTagClose(source, tagStart);
            if (tagEnd === -1) { out += source.slice(tagStart); break; }
            const tag = source.slice(tagStart, tagEnd + 1);
            const closeIdx = source.indexOf("</script>", tagEnd + 1);
            if (closeIdx === -1) { out += source.slice(tagStart); break; }
            const body = source.slice(tagEnd + 1, closeIdx);
            if (/\blang="ts"/i.test(tag)) {
              const transpiled = tsTranspiler.transformSync(body);
              out += tag + transpiled + "</script>";
            } else {
              out += tag + body + "</script>";
            }
            idx = closeIdx + 9;
          }
          source = out;
        }

        let result;
        try {
          result = compile(source, { generate, filename: args.path });
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
