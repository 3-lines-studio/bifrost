await (async () => {
  const babel = (await import("@babel/core")).default;
  const BabelPluginReactCompiler = (await import("babel-plugin-react-compiler")).default;
  return {
    name: "react-compiler",
    setup({ onLoad }) {
      onLoad({ filter: /\.[jt]sx$/ }, async (args) => {
        const input = await Bun.file(args.path).text();
        const result = await babel.transformAsync(input, {
          filename: args.path,
          plugins: [[BabelPluginReactCompiler, {}]],
          parserOpts: { plugins: ["jsx", "typescript"] },
          ast: false,
          sourceMaps: false,
          configFile: false,
          babelrc: false,
        });
        if (result?.code == null) {
          throw new Error(`Failed to compile ${args.path}`);
        }
        return { contents: result.code, loader: "tsx" };
      });
    },
  };
})()
