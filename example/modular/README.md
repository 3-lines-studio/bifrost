# Bifrost modular example

A reference layout for a production Go + Bifrost web app using **domain/modular
architecture** instead of hexagonal clean architecture. Each business capability
is one self-contained package that owns its model, service, repo interface,
store adapter, HTTP routes, Bifrost page loaders, and async tasks. Cross-cutting
plumbing lives in leaf modules that carry no business logic.

This example compiles, vets, and tests offline. It uses asynq for the queue,
pgx/v5 for Postgres, and stubs for storage, mailer, and i18n so no external
service is needed to build or test.

```
cmd/server/main.go        # the only wiring point
internal/
  web/                    # shared JSON writer, error mapper, task decoder
  config/                 # leaf: env + secrets, validated, fail-fast
  db/                     # leaf: pgx pool, Tx helper
  queue/                  # leaf: asynq client, Enqueue
  storage/                # leaf: S3 handle (stub)
  mailer/                 # leaf: SMTP handle (stub)
  i18n/                   # leaf: message catalog (stub)
  user/                   # domain module
  billing/                # domain module, depends on user
  notify/                 # domain module, depends on queue + i18n
  app/                    # composition root: mounts bifrost, http mux, asynq mux
pages/                    # React source (Vite owns this, not Go)
```

## The mental model

- **Instance, then wire, then run.** `main` allocates every module with `New()`,
  then `Wire()`s them in dependency order, then hands them to `app.New`, then
  runs `app.Run`.
- **Every module is a `Module` struct with `New()` and `Wire(*deps)`.** `New`
  allocates with no dependencies. `Wire` injects them. Between the two the
  module is invalid, so `main` never uses a module before wiring it.
- **Exporting nothing but the module.** A module does not export its repo
  interface (it declares it privately, beside the consumer), its store adapter,
  or HTTP helpers. Only the `Module` type and its public domain methods go out.
- **`config` is a module too.** Prod configs are complex; give them the same
  `New`/`Wire` shape and keep them a leaf that everything else reads via
  `Value()`.
- **The composition root is the only place muxes are created.** `app` collects
  every module's `Pages()`, `RegisterHTTP`, `RegisterTasks`, and `Run` into one
  bifrost app, one `http.ServeMux`, and one asynq `ServeMux`.

## The module contract

Every domain module implements the app surface implemented by `app.Module`:

```go
type Module interface {
    RegisterHTTP(mux *http.ServeMux)   // REST routes, zero business logic
    Pages() []bifrost.Route            // SSR page loaders, zero business logic
    RegisterTasks(mux *asynq.ServeMux) // async task handlers
    Run(context.Context) error         // background loop (cron etc.), or nil
}
```

Capabilities a module lacks are one-line no-ops, so the shape never changes. The
module surface is **the only seam** in the architecture. The Muxes are assembled
once, in `app.New`.

## Module internals

A module is a single package, files split by the conventional concern. A module
that needs no persistence omits `model.go`, `repo.go`, and `store.go` (`notify`).
A module that needs no background work still keeps a one-line `Run` so the surface
is uniform.

```
user/
  model.go    # the aggregate, plus web sentinel aliases where used
  repo.go     # unexported narrow port interface (next to the consumer)
  store.go    # private store adapter implementing the port
  service.go  # Module type, New/Wire, domain methods
  handler.go  # RegisterHTTP, thin REST handlers
  pages.go    # Pages(), thin bifrost loaders
  tasks.go    # RegisterTasks, async handlers, Run
```

The repo interface is unexported and lives beside its consumer. Its single
implementation sits next to it in `store.go`. Export the interface only if a
second implementation appears.

### Shared web kit

Every module reuses `internal/web` for transport, so there is no repeated JSON
or error-mapping code:

- `web.WriteJSON(w, status, v)` — the one success writer.
- `web.WriteError(w, err)` — the one mapper. It classifies by `errors.Is` against
the shared sentinels (`web.ErrNotFound`, `web.ErrConflict`, etc.).
- `web.DecodeJSON(r, v)` / `web.DecodeTask(payload, v)` — bounded decoders.

Modules return `web.ErrNotFound` (optionally wrapped with `%w` for context); they
never define their own sentinel set. The transport layer reads those through the
shared writer, so success and error responses stay uniform across modules.

## Dependency direction

The DAG is explicit and acyclic. Leaves import only `config`. Domain modules
import leaves and one another, but never in a cycle, and never down into a
shared model or handler package. `billing` may import `user` and call its public
methods; `user` never imports `billing`.

```
config
  └─ db, queue, storage, mailer, i18n     (leaves)
       └─ user                            (depends on db, i18n)
            └─ billing                    (depends on db, queue, storage, i18n, user)
                └─ notify                 (depends on queue, mailer, i18n)
app  (composition root)  ← everything not a leaf
```

## The bifrost build-phase guard

`app.New` and every module `New`/`Wire` construct handles only; they never dial,
connect, or listen. That keeps the Bifrost describe/generate phases working
(which need the page routes declared) without opening Postgres, Redis, or a
listener. `main` returns immediately when `bifrost.Building()` is true, after
wiring but before `Run`.

```go
if bifrost.Building() {
    return
}
```

## Running

```sh
cd example/modular
go run ./cmd/server
```

Set the config env vars before running (see `internal/config`). The production
manifest is generated by `bun run ../cmd/bifrost build ./cmd/server`; the
embedded stub in `cmd/server/.bifrost` only exists to satisfy `go:embed` during
development.

## Tests

`internal/app` has a composition test that asserts the module DAG wires into the
http mux patterns without touching Postgres or Redis. It verifies the build-phase
describe path works (spec written to `BIFROST_FD`).
