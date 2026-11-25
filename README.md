# Pym

> Named after **Dr. Hank Pym**, the Marvel scientist who invented the particles that
> shrink things to ant-size. Fitting, for a URL shortener.
>
> **Tagline:** _"Powered by Pym Particles."_

A small, boring-on-purpose URL shortener written in Go. Built as a take-home for an
Apple contractor interview, so it optimizes for the things a reviewer actually cares
about: clear boundaries, a real database, graceful shutdown, health checks, a
distroless container, and a k8s manifest that runs as non-root. No frameworks of the
month. No clever tricks.

## What it does

- `POST /api/v1/shorten` — takes `{"url": "https://..."}`, returns a 7-character
  code and the full short URL.
- `GET /:code` — 302-redirects to the original URL.
- `GET /healthz` — liveness. Always 200 if the process is up.
- `GET /readyz` — readiness. 200 only if Postgres responds to a ping within 500 ms.

Codes are generated from `crypto/rand` over a 62-char alphabet (≈62⁷ ≈ 3.5 × 10¹²
combinations). On a collision — detected via Postgres unique-violation `23505` — the
service retries up to 5 times before giving up, so no one is stuck waiting on a
hot code.

## Layout

```
.
├── main.go                           # wiring: env, logger, store, server, signals
├── internal/
│   ├── server/       server.go       # Echo v5 handlers + middleware
│   └── shortener/
│       ├── shortener.go              # Service, MemoryStore, code generation, URL validation
│       └── pgstore/  pgstore.go      # Postgres-backed Store (pgxpool)
├── Dockerfile                        # distroless, nonroot, static binary
├── docker-compose.yml                # just Postgres, for local dev
└── k8s/                              # Deployment, Service, ConfigMap, Secret
```

The `Store` interface lives in `internal/shortener/shortener.go` so the service
doesn't depend on Postgres — `MemoryStore` is a drop-in for tests or a laptop run
without Docker.

## Running it

### Locally with Docker Compose (Postgres only)

```bash
git clone git@github.com:1995parham-learning/Pym.git
cd Pym
docker compose up -d postgres
go run .
```

Defaults (override with env vars):

| Var            | Default                                                 |
| -------------- | ------------------------------------------------------- |
| `ADDR`         | `:8080`                                                 |
| `BASE_URL`     | `http://localhost:8080`                                 |
| `DATABASE_URL` | `postgres://pym:pym@localhost:5432/pym?sslmode=disable` |

### As a container

```bash
docker build -t pym:latest .
docker run --rm -p 8080:8080 \
  -e DATABASE_URL='postgres://pym:pym@host.docker.internal:5432/pym?sslmode=disable' \
  pym:latest
```

### On Kubernetes

```bash
kubectl apply -f k8s/
```

The manifest assumes a `pym:latest` image is reachable to the cluster (load
it into kind/minikube or push to a registry first). It runs 2 replicas as UID
65532, read-only rootfs, all capabilities dropped, and wires liveness/readiness to
`/healthz` and `/readyz`.

## Trying it

```bash
# Shorten
curl -sX POST localhost:8080/api/v1/shorten \
  -H 'content-type: application/json' \
  -d '{"url":"https://www.apple.com/newsroom/"}'
# → {"code":"aK3f9Zp","short_url":"http://localhost:8080/aK3f9Zp"}

# Follow
curl -sI localhost:8080/aK3f9Zp
# → HTTP/1.1 302 Found
#   Location: https://www.apple.com/newsroom/
```

## Design notes (a.k.a. "why does it look so plain")

- **One table, no migrations tool.** The schema is a single `CREATE TABLE IF NOT
EXISTS` applied at boot. There's a `TODO` in `pgstore.go` to swap in
  `golang-migrate` the moment a second table or an `ALTER` shows up — adding it now
  would be ceremony without payoff.
- **Collision handling at the DB, not in app memory.** The app doesn't pre-check
  whether a code exists; it inserts and lets the unique index reject duplicates.
  One round-trip on the happy path, correct under concurrency.
- **URL validation is deliberately narrow.** Only `http`/`https` with a non-empty
  host. Anything else is a 400. No SSRF protection beyond scheme — that's a
  deployment-level concern (egress policy), not something to half-do here.
- **`readyz` is separate from `healthz`.** Liveness shouldn't fail because Postgres
  is slow — that would make Kubernetes restart a healthy pod. Readiness is the
  knob that takes the pod out of the Service endpoints.
- **Graceful shutdown.** `main` wires `SIGINT`/`SIGTERM` into a context that
  Echo's `StartConfig` honors, with a 10 s drain. The pg pool closes after the
  server returns.

## What I'd add next

- Tests (table-driven for `normalizeURL`, integration tests against a real
  Postgres via testcontainers).
- Structured request IDs threaded through `slog`.
- Per-IP rate limit on `/api/v1/shorten`.
- Metrics (`/metrics` with `prometheus/client_golang`).
- A `golang-migrate` setup the first time the schema moves.

## License

Unlicensed — written as an interview exercise. Ask before reusing.
