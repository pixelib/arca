---
title: Managing an installation
layout: default
nav_order: 7
---

# Managing an installation

Everything on this page lives under **Manage** in the sidebar, and needs an administrator account.

## Repositories

![Managing repositories]({{ site.baseurl }}/assets/img/manage-repositories.jpg)

Creating a repository asks for four things that cannot all be changed later:

- **Format**, Maven 2 or npm. Fixed once the repository exists, because nothing migrates between
  formats.
- **Type**, hosted or proxy. A proxy needs a remote URL and is read only.
- **Policy**, releases only, prereleases only, or mixed. Changing it later does not remove versions
  that no longer fit, it only rejects new ones.
- **Visibility**, public or private. Public repositories are readable without an account.

`allowRedeploy` lets a hosted Maven repository overwrite an existing release. It is off by default,
and it does not apply to npm, where a published version is always immutable.

## Proxy repositories

![A proxy repository]({{ site.baseurl }}/assets/img/proxy.jpg)

A proxy mirrors a remote and caches what it serves. The first request for a path fetches it, stores
it like any other artifact, and serves it from disk after that.

Three timers control the cache:

| Setting | Applies to |
| --- | --- |
| `metadataTtlSeconds` | `maven-metadata.xml`, non timestamped snapshots and npm packuments |
| `negativeTtlSeconds` | paths the remote does not have |
| `cacheRetentionDays` | eviction of files nobody has downloaded |

The negative cache matters more than it sounds. Maven and Gradle ask for large numbers of
`-sources.jar`, `-javadoc.jar` and parent POMs that were never published, and without it every build
re-asks the remote for all of them.

Released artifacts and timestamped snapshot builds are cached until evicted, because Maven treats
them as immutable. When a cached path expires the next request revalidates with `If-None-Match` and
`If-Modified-Since`, so an unchanged file costs a 304 rather than a download. If the remote is
unreachable, the stale copy is served instead of an error.

Set `cacheRetentionDays` to evict automatically. A cached file nobody has downloaded in that many
days is deleted from disk and from the database. Zero, the default, keeps everything until you purge
by hand from the repository's settings dialog.

## Users and access

A user is either an administrator or a regular user. Administrators see everything and can manage
the instance. Everyone else sees public repositories plus whatever they have been granted, per
repository, as read, write or admin.

API tokens are created by each user under Account. Only a hash is stored, so a token is shown once
and cannot be recovered, only replaced. Tokens are the right credential for builds: they are
verified with a single hash, while a password runs PBKDF2 on every request.

## Categories

Categories group repositories in the sidebar and on the index without touching their URLs. They are
presentation only. Deleting one leaves its repositories in place, uncategorised.

## Health

![The health dashboard]({{ site.baseurl }}/assets/img/health.jpg)

Traffic is recorded per request and rolled up over the window you pick. Downloads, uploads and
visits are counted separately, so a spike in visits from a crawler does not look like build traffic.

The runtime numbers next to them are the Go process: uptime, goroutines and heap. They are the
quickest way to tell a busy instance from a stuck one.

## Storage

![The storage dashboard]({{ site.baseurl }}/assets/img/storage.jpg)

Storage is broken down by repository, by file type and by the heaviest individual versions, with
releases and snapshots split out.

**Untracked** is the number to watch. It is bytes on disk that no database row points at, which
should be zero. Anything else means files were left behind, usually by an interrupted delete.

## Deleting artifacts

Artifacts are deleted from the artifact page, from the Artifacts screen under Manage, or over the
API. A delete removes the files from disk and the rows from the database, and the component row goes
with them once no files are left under that version.

Deleting from a proxy only drops the cached copy. The next request fetches it again.

## Backups

Copy the storage directory and the SQLite database. Stop the process first, or use `sqlite3 .backup`
so the database is copied consistently while it is running. Copying a live database file with `cp`
can capture a torn write.
