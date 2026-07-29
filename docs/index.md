---
title: Home
layout: default
nav_order: 1
---

# Arca

An artifact repository server that runs as a single Go binary. It speaks Maven 2, npm and Eclipse p2,
stores artifacts on disk, keeps metadata in SQLite, and serves an embedded React UI from the same
process. There is no separate database to run, no application server, and no config file to write
before the first start.

![The repository index]({{ site.baseurl }}/assets/img/repositories.jpg)

## What it does

- **Three formats.** Maven 2, npm and Eclipse p2, chosen per repository and fixed once it exists.
- **Hosted and proxy repositories.** Publish to a hosted repository, or mirror Maven Central,
  registry.npmjs.org and download.eclipse.org through a proxy that caches what it serves.
- **Version policies.** A repository takes releases only, prereleases only, or both.
- **Per user permissions.** Public repositories are readable without an account. Private ones are
  granted per user, per repository.
- **Generated metadata.** `maven-metadata.xml` and npm packuments are built from the database, so
  they cannot drift from what is actually stored.
- **Insights.** Traffic, storage and per version artifact management, with the numbers coming from
  real requests rather than estimates.

## Where to start

| If you want to | Read |
| --- | --- |
| Run it locally and look around | [Installing]({{ site.baseurl }}/installing) |
| Resolve and publish jars | [Using it from Maven]({{ site.baseurl }}/maven) |
| Resolve and publish packages | [Using it from npm]({{ site.baseurl }}/npm) |
| Resolve Eclipse bundles and features | [Using it from Eclipse]({{ site.baseurl }}/p2) |
| Put it on a server | [Deploying]({{ site.baseurl }}/deploying) |
| Run it day to day | [Managing an installation]({{ site.baseurl }}/managing) |

## Finding things

Search spans every repository you can read, in any format, matching names and namespaces.

![Search results]({{ site.baseurl }}/assets/img/search.jpg)

## How it fits together

One process serves three things on the same port:

| Path | Serves |
| --- | --- |
| `/repository/<name>/**` | the Maven or npm endpoint, depending on the repository format |
| `/api/**` | the JSON API the UI runs on |
| everything else | the embedded React bundle |

Uploads are streamed to disk while MD5, SHA-1, SHA-256 and SHA-512 are computed in one pass, so an
artifact is bounded by disk rather than memory. SQLite means one writer at a time, which is fine for
a single instance but rules out several replicas sharing a database file.
