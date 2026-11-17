---
title: Single sign-on
layout: default
nav_order: 10
---

# Single sign-on

Arca can sign people in through an OpenID Connect provider such as Authentik, Keycloak or Entra ID.
Setting `OIDC_ISSUER` turns it on: the login page gains a **Sign in with ...** button next to the
password form, which keeps working for accounts that have a password.

## Configuration

| Flag | Environment | Default | Purpose |
| --- | --- | --- | --- |
| `--oidc-issuer` | `OIDC_ISSUER` | empty, off | issuer URL of the provider |
| `--oidc-client-id` | `OIDC_CLIENT_ID` | | client id, required with an issuer |
| `--oidc-client-secret` | `OIDC_CLIENT_SECRET` | | client secret, required with an issuer |
| `--oidc-name` | `OIDC_NAME` | `SSO` | provider name on the sign-in button |
| `--oidc-scopes` | `OIDC_SCOPES` | `openid profile email` | space-separated scopes to request |
| `--oidc-groups-claim` | `OIDC_GROUPS_CLAIM` | `groups` | ID token claim that lists the user's groups |
| `--oidc-admin-groups` | `OIDC_ADMIN_GROUPS` | empty | comma-separated groups whose members are administrators |
| `--oidc-allowed-groups` | `OIDC_ALLOWED_GROUPS` | empty, everyone | comma-separated groups allowed to sign in |
| `--oidc-link-existing` | `OIDC_LINK_EXISTING` | `false` | link a first sign-in to an existing account with the same email |

Register this redirect URI with the provider:

```
https://artifacts.example.com/api/auth/oidc/callback
```

It is built on `BASE_URL`, or on the address of the request when that is not set. The email, name
and groups are read from the ID token, so the provider has to put them there; an ID token without an
email is refused.

Accounts are matched on the provider's subject, so a user whose email changes at the provider keeps
their account. Only when no account is linked yet does the email matter: a free address gets a new
account, and one that already belongs to an account is refused unless `OIDC_LINK_EXISTING` is on.
Turn that on only for a provider that verifies email addresses, since it lets whoever holds that
address at the provider into the account. To limit who gets an account, use `OIDC_ALLOWED_GROUPS`.

The display name comes from the provider when the account is created and is not synced afterwards,
so a name changed in Arca stays. The email is updated at every sign-in, unless another account
already uses the new address.

## Setting up with single sign-on

With OIDC configured, the administrator step of the first-run wizard offers **Continue with ...**
next to the name, email and password fields. Signing in there makes that provider account the
administrator, so an instance can be set up and managed without any password account. The setup
token is still required.

With `OIDC_ADMIN_GROUPS` set, only a member of one of those groups can set the instance up through
the provider. Anyone else is turned away and can still use the password fields.

Until setup is finished, sign-in through the provider creates no accounts: it only fills in the
wizard's administrator step.

## Authentik example

In Authentik, create an **OAuth2/OpenID Provider** with a confidential client, set its redirect URI
to the one above, keep **Include claims in id_token** on, and add an application for it. The
default `profile` scope already carries a `groups` claim.

```sh
OIDC_ISSUER=https://auth.example.com/application/o/arca/
OIDC_CLIENT_ID=...
OIDC_CLIENT_SECRET=...
OIDC_NAME=Authentik
OIDC_ADMIN_GROUPS=arca-admins
OIDC_ALLOWED_GROUPS=arca-users,arca-admins
```

The issuer is the application's **OpenID Configuration Issuer**, trailing slash included.

## Things to know

When admin groups are set, the provider owns the role of every account linked to it. Every sign-in
makes a member of an admin group an administrator and everyone else a user, and the role of a linked
account cannot be changed under **Manage**. The one exception is the last enabled administrator, who
is never demoted. Accounts that never signed in through the provider keep a role set in Arca. With
`OIDC_ADMIN_GROUPS` empty, Arca never touches a role.

Removing someone at the provider only takes effect at their next sign-in. Sessions last 30 days and
API tokens do not expire, so to offboard someone, disable their user in Arca as well.

Build tools keep using API tokens. Maven, npm and Docker clients cannot follow a browser sign-in,
and an account created through single sign-on has no password, so it creates an API token under
**Account** and uses that.
