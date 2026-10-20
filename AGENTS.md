# AGENTS.md

Instructions for coding agents working on Bifrost. User-facing docs are in `README.md`; do not add more.

## What this is

Bifrost is a file-based React App Router served by Go. Go owns routing, loaders, HTTP and the build pipeline. Bun renders React on the server and runs Vite. This repository contains the framework, the `bifrost` CLI and a reference app.

## Setup

Go 1.25+ and Bun 1.3+.

## Commands

- `make test`: gofmt check and all Go tests. Run before every push.
- `make vet`: `go vet ./...`.
- `make gate`: App Router release gate; builds a throwaway app and exercises it (~10s).
- `make demo`: builds `example/app-router-demo` and runs 74 curl assertions.
- `make dev-integration`: development loop; HMR, Go restart, Vite bridge, route add and remove.
- `make check`: test, race, vet, gate and demo.
- `make integration`: every integration script; slower and needs the full environment.
- `go run ./cmd/bifrost <build|dev|init|routes> ...`: the CLI at this tree.
- `cd example/app-router-demo && make dev`: App Router development server on `:8080`.

`make test` is the minimum. Add `make gate` and `make demo` when the CLI, the builder, routing or the App Router changes.

## Layout

- `cmd/bifrost/`: CLI commands and the App Router generator.
- `*.go` at the root: runtime, routes, rendering, HTTP, manifests.
- `internal/builder/`: Vite and Bun build pipeline, manifests.
- `internal/renderproc/`: renderer processes and the embedded TypeScript runtime.
- `internal/dochtml/`, `internal/protocol/`: document HTML and the IPC model.
- `scripts/`: release gate and integration scripts.
- `example/app-router-demo/`: the reference App Router app.

## Rules

- Branch from `origin/dev` (`git fetch && git reset --hard origin/dev`), one logical change per branch, PR against `dev`.
- Only the maintainer merges pull requests. Open the PR and stop.
- Code and docs in English. Commits in imperative English.
- No comments in Go, TypeScript, shell or CSS. The only allowed comments are Go directives (`//go:build`, `//go:embed`).
- No documentation beyond `README.md` and this file.
- Reproduce the bug first, then show the same check passing.
- Do not touch `.bifrost/`: it is build output.
- Add no dependency unless the change needs it.

## App Router invariants

- Route markers: `slug_` is one segment, `slug__` is a catch-all and must be last, `~` groups without changing the URL, a leading `_` keeps a directory out of routing, bracketed names fail the build.
- Only `page.tsx` may export `Head`.
- A loader returns a map or `bifrost.PageData` whose `Props` is a map.
- `zz_bifrost_gen.go` and everything under `.bifrost/` are generated.

## Loop

1. Run the check that reproduces the problem.
2. Make the smallest change.
3. Run that check again, then `make test`.
4. For a live loop, run `cd example/app-router-demo && make dev` and edit; the server reloads routes and rebuilds the Go app. `bifrost routes --json` prints the route table for tooling.
