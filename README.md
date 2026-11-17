<p align="center">
<img src="docs/assets/img/logo.png"/>
</p>

# Arca [![Release](https://github.com/pixelib/arca/actions/workflows/release.yml/badge.svg)](https://github.com/pixelib/arca/actions/workflows/release.yml) [![CI](https://github.com/pixelib/arca/actions/workflows/ci.yml/badge.svg)](https://github.com/pixelib/arca/actions/workflows/ci.yml)
A Maven, npm, Docker and Eclipse p2 repository server that runs as a single Go binary.

![Arca](docs/assets/img/repositories.png)

Artifacts live on disk, metadata lives in SQLite, and the React UI is embedded in the binary. There
is no separate database to run and no config file to write before the first start.

- Maven 2, npm and Docker repositories, hosted or proxying a remote
- Eclipse p2 update sites, hosted or proxied and cached, with composite groups for PDE and Tycho
- Container images pushed and pulled with any Docker client, with layers, platforms and build history
  in the UI
- Release, prerelease and mixed version policies
- Public repositories readable without an account, private ones granted per user
- Single sign-on through any OpenID Connect provider, with group-based access and admin roles
- Deployment links: a fixed URL that downloads the newest matching file of an artifact, without
  authentication, even from a private repository
- Repositories can be renamed, with the old name left redirecting until something else claims it
- Generated `maven-metadata.xml` and npm packuments, built from the database
- Traffic, storage and artifact management dashboards
- A markdown welcome hero, an accent colour and a logo of your own on the landing page
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

Open <http://localhost:8080> and complete the setup wizard. It asks for the setup token the
server printed to its log on startup, so nobody else can claim a fresh instance. There is a
[docker compose example](https://pixelib.github.io/arca/deploying#docker) in the docs.

## Docs

**[pixelib.github.io/arca](https://pixelib.github.io/arca)** covers using it from Maven,
npm and Docker, deploying it, and running an installation.

A live instance is at [repo.pixelib.dev](https://repo.pixelib.dev).

## Develop

```sh
make dev        # vite on :5173, which starts the Go server on :8080 in dev mode
make test       # go tests plus a frontend typecheck
```


## Need help?
[Join our Discord!](https://discord.gg/cep5gXDVHf) Our lovely developer and friends will try to help you out.

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
| `internal/docker` | image names, tags, digests, the storage layout and manifest parsing |
| `internal/proxy` | upstream HTTP client and fetch deduplication |
| `internal/blob` | artifact storage on disk |
| `internal/frontend` | React app plus the `go:embed` handler |
