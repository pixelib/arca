<p align="center">
<img src="docs/assets/img/logo.png"/>
</p>

# Arca [![Release](https://github.com/pixelib/arca/actions/workflows/release.yml/badge.svg)](https://github.com/pixelib/arca/actions/workflows/release.yml) [![CI](https://github.com/pixelib/arca/actions/workflows/ci.yml/badge.svg)](https://github.com/pixelib/arca/actions/workflows/ci.yml)
A Maven, npm and Eclipse p2 repository server that runs as a single Go binary.

![Arca](docs/assets/img/repositories.jpg)

Artifacts live on disk, metadata lives in SQLite, and the React UI is embedded in the binary. There
is no separate database to run and no config file to write before the first start.

- Maven 2 and npm repositories, hosted or proxying a remote
- Eclipse p2 update sites, hosted or proxied and cached, for PDE and Tycho
- Release, prerelease and mixed version policies
- Public repositories readable without an account, private ones granted per user
- Generated `maven-metadata.xml` and npm packuments, built from the database
- Traffic, storage and artifact management dashboards
- Guided first-run setup

## Run it

```sh
docker run -d -p 8080:8080 -v arca-data:/data ghcr.io/pixelib/arca:latest
```

Or grab a binary for Linux, macOS or Windows from the
[latest release](https://github.com/Pixelib/arca/releases/latest), or build it:

```sh
make build      # builds the frontend, then the binary into bin/arca
./bin/arca
```

Open <http://localhost:8080> and complete the setup wizard. There is a
[docker compose example](https://pixelib.github.io/arca/deploying#docker) in the docs.

## Docs

**[pixelib.github.io/arca](https://pixelib.github.io/arca)** covers using it from Maven
and npm, deploying it, and running an installation.

A live instance is at [repo.pixelib.dev](https://repo.pixelib.dev).

## Develop

```sh
make dev        # vite on :5173, which starts the Go server on :8080 in dev mode
make test       # go tests plus a frontend typecheck
```

## Layout

| Path | Contents |
| --- | --- |
| `cmd/arca` | entry point |
| `internal/server` | HTTP routing, JSON API, format endpoints, UI mounting |
| `internal/store` | GORM models and queries |
| `internal/format` | the format seam: coordinates, layouts, version ordering |
| `internal/maven` | Maven coordinates, version ordering, metadata rendering |
| `internal/npm` | npm routes, semver, packuments, publish parsing |
| `internal/p2` | p2 paths, OSGi versions, artifact document rewriting |
| `internal/proxy` | upstream HTTP client and fetch deduplication |
| `internal/blob` | artifact storage on disk |
| `internal/frontend` | React app plus the `go:embed` handler |
