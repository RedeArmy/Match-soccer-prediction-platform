# World Cup Quiniela API

REST API for managing World Cup prediction pools built with Go.

## Branching strategy

| Branch    | Purpose                  | Merge policy                                |
|-----------|--------------------------|---------------------------------------------|
| `main`    | Production               | PRs from `develop` only - CI must pass      |
| `develop` | Integration / staging    | PRs from feature branches - CI must pass    |

Feature branches must be merged into `develop` first. Direct PRs from feature
branches to `main` are rejected by the CI pipeline.

## CI/CD

Every PR to `develop` or `main` runs:
- `go vet`
- Full test suite with race detection
- SonarCloud quality gate

Merges to `main` additionally trigger a Docker image build and push to
the GitHub Container Registry (`ghcr.io`).

## Requirements

- Go 1.26+
- Docker + Docker Compose

## Quick start

```bash
cp .env.example .env
make docker-up
make migrate
make hooks   # install git hooks (auto-regenerates Swagger docs on commit)
make run
```

> **Note:** `make hooks` must be run once after cloning. It installs the pre-commit hook
> that auto-regenerates `docs/` when handler annotations change, so the CI Swagger
> staleness check never blocks your PR.
