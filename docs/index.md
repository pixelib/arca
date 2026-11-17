---
title: Home
layout: default
nav_order: 1
---

# Arca

An artifact repository server that runs as a single Go binary. It speaks Maven 2, npm, Docker and
Eclipse p2, stores artifacts on disk, keeps metadata in SQLite, and serves an embedded React UI from
the same process. There is no separate database to run, no application server, and no config file to write
before the first start.

![The repository index]({{ site.baseurl }}/assets/img/repositories.png)

## What it does

- **Four formats.** Maven 2, npm, Docker and Eclipse p2, chosen per repository and fixed once it
  exists.
- **Hosted, proxy and group repositories.** Publish to a hosted repository, mirror Maven Central,
  registry.npmjs.org, Docker Hub and download.eclipse.org through a proxy that caches what it serves,
  or put a p2 group over several update sites so a target platform needs one location instead of a
  dozen.
- **Version policies.** A repository takes releases only, prereleases only, or both.
- **Per user permissions.** Public repositories are readable without an account. Private ones are
  granted per user, per repository.
- **Deployment links.** A fixed URL that always downloads the newest matching file of an artifact,
  without authentication, so a private repository can publish exactly one file to an installer or a
  build that has no credentials.
- **Repositories can be renamed.** The old name keeps redirecting, until another repository claims
  it.
- **Generated metadata.** `maven-metadata.xml` and npm packuments are built from the database, so
  they cannot drift from what is actually stored.
- **Container images as first-class artifacts.** Push and pull with any Docker client, then read the
  layers, platforms, build history and how much of a tag is unique to it rather than shared.
- **Insights.** Traffic, storage and per version artifact management, with the numbers coming from
  real requests rather than estimates.
- **Your own front page.** A markdown welcome text, an accent colour and a logo of your own, so the
  landing page explains what the instance is for instead of listing repositories at a stranger.

## Where to start

| If you want to | Read |
| --- | --- |
| Run it locally and look around | [Installing]({{ site.baseurl }}/installing) |
| Resolve and publish jars | [Using it from Maven]({{ site.baseurl }}/maven) |
| Resolve and publish packages | [Using it from npm]({{ site.baseurl }}/npm) |
| Resolve Eclipse bundles and features | [Using it from Eclipse]({{ site.baseurl }}/p2) |
| Push and pull container images | [Using it from Docker]({{ site.baseurl }}/docker) |
| Move off Sonatype Nexus | [Migrating from Nexus]({{ site.baseurl }}/migrating) |
| Put it on a server | [Deploying]({{ site.baseurl }}/deploying) |
| Run it day to day | [Managing an installation]({{ site.baseurl }}/managing) |
| Sign in through your identity provider | [Single sign-on]({{ site.baseurl }}/oidc) |

## Need help?
[Join our Discord!](https://discord.gg/cep5gXDVHf) Our lovely developer and friends will try to help you out.

## Finding things

Search spans every repository you can read, in any format, matching names and namespaces.

![Search results]({{ site.baseurl }}/assets/img/search.jpg)

## How it fits together

One process serves four things on the same port:

| Path | Serves |
| --- | --- |
| `/repository/<name>/**` | the Maven, npm or p2 endpoint, depending on the repository format |
| `/v2/**` | the Docker registry API, which clients build their own URLs for and so cannot be given a prefix |
| `/api/**` | the JSON API the UI runs on |
| everything else | the embedded React bundle |

Uploads are streamed to disk while MD5, SHA-1, SHA-256 and SHA-512 are computed in one pass, so an
artifact is bounded by disk rather than memory. SQLite means one writer at a time, which is fine for
a single instance but rules out several replicas sharing a database file.
