# Formlander

[![CI](https://github.com/karloscodes/formlander/actions/workflows/ci.yml/badge.svg)](https://github.com/karloscodes/formlander/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/karloscodes/formlander)](https://github.com/karloscodes/formlander/releases/latest) [![Docker pulls](https://img.shields.io/docker/pulls/karloscodes/formlander)](https://hub.docker.com/r/karloscodes/formlander) [![License: MIT](https://img.shields.io/github/license/karloscodes/formlander)](LICENSE)

Form backend you actually own.

Formlander takes the form posts of your static or serverless sites on your own server. It stores each submission in one SQLite file, shows it in an admin dashboard, and delivers it to a webhook or by email. No SaaS bills, and nobody else sees your submissions.

**[Website](https://formlander.com)** · **[Docs](https://formlander.com/docs/)**

## Install

On a fresh Ubuntu or Debian server, as root:

```bash
curl -fsSL https://formlander.com/install | sudo bash -s forms.example.com
```

It installs [Chasen](https://chasenhq.com) (and Docker, when the server has none) and runs Formlander: HTTPS, live backups of the database, and an update each night. Run it again to update now. The server needs ports 80 and 443 open, and an A record for your domain.

Sign in at `https://forms.example.com` as `admin@formlander.local`. The installer prints the first password; change it in Settings. To read it again: `chasen -a formlander run cat /app/storage/initial-admin-password`.

From your computer instead:

```bash
curl -fsSL https://chasenhq.com/cli | sh
chasen deploy karloscodes/formlander --server root@203.0.113.5 --domain forms.example.com --auto-update
```

Servers that the older installer set up keep working as they are. The [deployment guide](https://formlander.com/docs/deployment/) has the details.

## Use

Point a form at your Formlander server:

```html
<form action="https://forms.example.com/forms/contact/submit?token=YOUR_TOKEN" method="post">
  <input name="email" type="email" required>
  <button type="submit">Send</button>
</form>
```

A plain HTML post works. The optional script adds retries with backoff on a 503 or a network error, and a "Sending..." state, and falls back to the normal post when JavaScript fails:

```html
<script src="https://forms.example.com/assets/formlander.js"></script>
```

## What you get

- **Admin dashboard** - Forms, submissions, and the delivery status of each one.
- **Delivery** - Webhooks and email (Mailgun), from a queue that retries.
- **Spam protection** - A honeypot field and rate limiting.
- **One container** - One Go binary and one SQLite file, with no other service.

## Configuration

On Chasen, Formlander needs no settings: Chasen gives it its session secret and keeps `/app/storage` across deploys. To run the image yourself:

```bash
docker run -d -p 127.0.0.1:8080:8080 \
  -e FORMLANDER_SESSION_SECRET="$(openssl rand -hex 32)" \
  -v "$PWD/storage:/app/storage" \
  karloscodes/formlander:latest
```

Keep the same `FORMLANDER_SESSION_SECRET` across restarts, or every session ends. The image runs in production mode, which marks the session cookie `Secure`: put HTTPS (Caddy, Nginx, Traefik) in front of it. Other settings: `FORMLANDER_PORT` (8080), `FORMLANDER_LOG_LEVEL` (`error`), `FORMLANDER_DATA_DIR` (`./storage`). The tags `latest`, `3.8.0`, `v3`, `v3.8`, and `rc` (the newest release candidate) are on Docker Hub.

## Development

```bash
make dev      # the dev server
make test     # the tests
make build    # the binary, in bin/formlander
make seed     # sample data
```

The code is in `internal/`: `forms`, `integrations`, `jobs` (delivery), `database`, `http`, and `accounts`.

## License

MIT. See [LICENSE](LICENSE).
