# Bifrost with Wails

This experiment mounts a client-only Bifrost application in a Wails v3 asset handler. Bifrost owns React and Vite. Wails owns the native window, generated Go bindings, lifecycle, and packaging boundary.

## Requirements

- Bun
- Go
- Wails v3 CLI beta 12
- Native Wails dependencies for the target platform

## Development

```sh
cd example/wails
wails3 dev
```

Wails owns the development process, Go rebuilds, binding generation, and window restarts. Bifrost prepares declarations, client entries, and a lightweight development manifest without running a Vite bundle build. One Vite process stays alive across Go changes.

## Build

```sh
cd example/wails
wails3 build
./bin/bifrost-wails
```

The production application contains Bifrost's client assets and no Bun renderer. The example builds a native executable but does not include installer, signing, or store configuration.

## Browser check

Wails server mode exposes the same asset handler and bindings without a native window:

```sh
cd example/wails
wails3 task server
```

Open `http://localhost:8080`. This mode is useful for browser automation, not as the native deployment target.

Run the production and external-Vite browser checks with Chromium:

```sh
wails3 task check
```

## Boundaries

Use Bifrost `Client` routes. Native application data should cross Wails bindings. `Server` routes would add a Bun production process, and mobile platforms cannot use that runtime model.

The external Vite server keeps React deduplication enabled because Bifrost's generated entry and the Wails frontend can resolve packages from different directories. Bifrost enforces the same rule in its own production and development builds.
