---
title: Deploying
layout: default
nav_order: 6
---

# Deploying

The build output is one static binary with the frontend embedded, so a deployment is: copy a file,
point it at a data directory, run it under a supervisor. Nothing else has to be installed on the
host.

The deployment scripts used for <https://repo.pixelib.dev> are deliberately not in this repository,
because an inventory naming real hosts does not belong in public source. This page has everything
needed to write your own, and the Ansible below is the same shape as the one in production.

## Docker

The quickest route. Images are published to the GitHub Container Registry on every release, for
`linux/amd64` and `linux/arm64`.

```sh
docker run -d --name arca \
  -p 8080:8080 \
  -v arca-data:/data \
  ghcr.io/pixelib/arca:latest
```

Or with compose:

```yaml
services:
  arca:
    image: ghcr.io/pixelib/arca:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - arca-data:/data
    environment:
      # 256 MiB. Raise it if you publish larger artifacts, and raise the body
      # size limit on whatever sits in front of it to match.
      MAX_UPLOAD_BYTES: "268435456"

volumes:
  arca-data:
```

To keep the data somewhere you can see it, swap the named volume for a bind mount and chown it once:

```sh
mkdir -p ./data && sudo chown -R 65532:65532 ./data
```

```yaml
    volumes:
      - ./data:/data
```

Everything lives in `/data`: the SQLite database at `/data/arca.db` and the artifacts under
`/data/storage`. Keep that volume and an upgrade is `docker compose pull && docker compose up -d`.

The image runs as the non root user `65532`. A named volume picks that up from the image, so it works
with no extra steps. A **bind mount** does not: Docker mounts the host directory with its own
ownership, so `chown -R 65532:65532 /your/host/path` first, otherwise the server exits at startup
with `/data is not writable by uid 65532`.

Pin a version rather than tracking `latest` if you want upgrades to be a decision:
`ghcr.io/pixelib/arca:1.2.0`, `:1.2` and `:1` are all published.

## Downloading a binary

Every release attaches archives for Linux, macOS and Windows on amd64 and arm64, with a
`checksums.txt` next to them. Unpack it, put the binary somewhere on the path, and run it.

```sh
arca --version
```

## Building it yourself

```sh
cd internal/frontend && pnpm install --frozen-lockfile && pnpm build
cd ../..
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/arca ./cmd/arca
```

`CGO_ENABLED=0` matters. It is what makes the binary static, so it runs on any Linux of the same
architecture regardless of the libc on the host.

To stamp the version into the build the way a release does:

```sh
go build -trimpath -ldflags "-s -w \
  -X arca/internal/version.Version=v1.2.0 \
  -X arca/internal/version.Commit=$(git rev-parse HEAD) \
  -X arca/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o build/arca ./cmd/arca
```

Without those flags the binary reports itself as `dev`, with the commit and time Go stamps in from
the git checkout. The running build is shown at the bottom of the sidebar.

## Run it under systemd

For a plain binary on a host. Copy it to `/opt/arca/arca`, create an `arca` system user, and give it
a data directory it owns.

```ini
[Unit]
Description=Artifact repository server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=arca
Group=arca
WorkingDirectory=/opt/arca
EnvironmentFile=/opt/arca/arca.env
ExecStart=/opt/arca/arca
Restart=on-failure
RestartSec=5
NoNewPrivileges=yes
PrivateTmp=yes
ProtectHome=yes
ProtectSystem=strict
ReadWritePaths=/opt/arca/data

[Install]
WantedBy=multi-user.target
```

With `/opt/arca/arca.env`:

```
PORT=8427
DATABASE_PATH=/opt/arca/data/arca.db
STORAGE_DIR=/opt/arca/data/storage
MAX_UPLOAD_BYTES=268435456
```

`ProtectSystem=strict` makes the whole filesystem read only except what `ReadWritePaths` lists, so
the data directory is the only thing the process can write. Keep it that way.

## Ansible

A playbook that does the whole thing. Adjust the variables at the top and run it against a host in
a `repo_servers` group.

```yaml
---
- name: Deploy Arca
  hosts: repo_servers
  become: yes

  vars:
    service_name: arca
    service_user: arca
    service_port: 8427
    install_dir: /opt/arca
    max_upload_bytes: 268435456

    binary_src: "{{ binary_path | default('build/arca') }}"
    data_dir: "{{ install_dir }}/data"
    storage_dir: "{{ data_dir }}/storage"
    database_path: "{{ data_dir }}/arca.db"
    environment_file: "{{ install_dir }}/arca.env"

  tasks:
    - name: Create system user
      ansible.builtin.user:
        name: "{{ service_user }}"
        system: yes
        shell: /sbin/nologin
        create_home: no

    - name: Create install directory
      ansible.builtin.file:
        path: "{{ install_dir }}"
        state: directory
        owner: root
        group: "{{ service_user }}"
        mode: "0750"

    - name: Create data directories
      ansible.builtin.file:
        path: "{{ item }}"
        state: directory
        owner: "{{ service_user }}"
        group: "{{ service_user }}"
        mode: "0750"
      loop:
        - "{{ data_dir }}"
        - "{{ storage_dir }}"

    - name: Copy binary
      ansible.builtin.copy:
        src: "{{ binary_src }}"
        dest: "{{ install_dir }}/arca"
        owner: root
        group: "{{ service_user }}"
        mode: "0750"
      notify: restart arca

    - name: Write environment file
      ansible.builtin.copy:
        dest: "{{ environment_file }}"
        content: |
          PORT={{ service_port }}
          DATABASE_PATH={{ database_path }}
          STORAGE_DIR={{ storage_dir }}
          MAX_UPLOAD_BYTES={{ max_upload_bytes }}
        owner: root
        group: "{{ service_user }}"
        mode: "0640"
      notify: restart arca

    - name: Install systemd unit
      ansible.builtin.template:
        src: arca.service.j2
        dest: "/etc/systemd/system/{{ service_name }}.service"
        owner: root
        group: root
        mode: "0644"
      notify:
        - reload systemd
        - restart arca

    - name: Enable service
      ansible.builtin.systemd:
        name: "{{ service_name }}"
        enabled: yes
        state: started
        daemon_reload: yes

    - name: Apply pending restarts before the health check
      ansible.builtin.meta: flush_handlers

    - name: Wait for the server to answer
      ansible.builtin.uri:
        url: "http://127.0.0.1:{{ service_port }}/api/bootstrap"
        status_code: 200
      register: bootstrap
      retries: 15
      delay: 2
      until: bootstrap is succeeded

  handlers:
    - name: reload systemd
      ansible.builtin.systemd:
        daemon_reload: yes

    - name: restart arca
      ansible.builtin.systemd:
        name: "{{ service_name }}"
        state: restarted
```

Two details worth keeping if you rewrite this:

- **`flush_handlers` before the health check.** Without it the check runs against the old process
  and passes while the new binary has not started, which turns a failed deployment into a green one.
- **`/api/bootstrap` as the check.** It answers before an administrator exists, so the same check
  works on a first deployment and on every one after.

## Releases

Tagging `v*` builds the binaries, attaches them to the GitHub release, and publishes the image. The
tag is what ends up in `arca --version` and in the sidebar, so it is worth getting right.

## Upgrading

Replace the binary and restart. The schema is applied on startup, and the data directory is never
touched by a deployment, so there is no migration step and no downtime beyond the restart.

On the first deployment there is no database. The server starts empty and serves the setup wizard,
so log in and complete it before pointing any builds at the host.

## TLS and reverse proxies

Terminating TLS is left to a reverse proxy. Two things to get right:

- Raise the proxy's body size limit to match `MAX_UPLOAD_BYTES`, or large artifacts fail at the
  proxy with an error that looks like it came from the repository.
- Forward `X-Forwarded-Proto` and `X-Forwarded-Host`. npm tarball URLs are built from the request,
  so without them a proxied npm repository hands out `http://` links to an internal hostname.

Serving the repository directly on a port, with no proxy in front, needs neither.
