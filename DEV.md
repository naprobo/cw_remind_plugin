# CW Remind Developer Guide

## Technology

- Go 1.24 server plugin
- React and TypeScript web application
- Mattermost server/public SDK `v0.1.12`
- Webpack production bundle
- Docker-based reproducible build environment

The manifest requires Mattermost 9.5 or later. Development and integration testing target Mattermost 10.12.

## Repository Layout

```text
assets/             Bot and channel-header icon
build/              Docker build and packaging script
server/             Go plugin, scheduler, HTTP API, and tests
webapp/src/         React UI, localization, and styles
plugin.json         Mattermost plugin manifest
Makefile            Native build targets
compose.yaml        Reproducible builder service
SPEC.md             Product and behavior specification
```

## Build

The recommended build requires only Docker:

```bash
docker compose run --rm builder
```

The builder formats and tests the Go server, compiles server binaries for all manifest platforms, checks TypeScript, builds the web application, and creates:

```text
dist/com.cw.remind-0.1.1.tar.gz
```

For a native build, install Go 1.24 or later, Node.js 20 or later, npm, GNU Make, and `tar`, then run:

```bash
make dist
```

## Verification

Run the server and web application checks separately with:

```bash
go test ./server/...
cd webapp
npm install
npm run check-types
npm run build
```

The archive must contain one top-level `com.cw.remind/` directory with `plugin.json`, platform server binaries, `webapp/dist/main.js`, `assets/`, and `public/`.

## Architecture

The server registers `/remind`, creates or reuses the `DueWatch` bot, exposes authenticated plugin APIs, stores reminders in the Plugin KV Store, and runs a 30-second scheduler. The web application registers the modal root component, channel-header action, WebSocket listener, localized UI, history view, and channel-member picker.

Executing `/remind` publishes a user-targeted WebSocket event. The web application opens the modal for the command channel and submits the reminder through the plugin API. The server validates the request, posts the initial announcement, stores the reminder, and later publishes pending notifications.

Interactive post actions contain a private token that is stored only in the internal reminder record. Public API responses remove this token.

## HTTP API

- `POST /api/v1/reminders` creates a reminder.
- `GET /api/v1/reminders?channel_id={id}` lists reminders for a channel, newest first.
- `GET /api/v1/channel-users?channel_id={id}&q={query}` searches active non-bot channel members.
- `PUT /api/v1/reminders/{id}` updates a reminder owned by the current user.
- `DELETE /api/v1/reminders/{id}` soft-deletes a reminder owned by the current user.
- `POST /api/v1/actions/acknowledge` processes the Acknowledged post action.
- `POST /api/v1/actions/complete` processes the Completed post action.

All GUI endpoints require the Mattermost-injected `Mattermost-User-ID`. The web application sends credentials, `X-Requested-With`, and the Mattermost CSRF token for JSON writes. The server always revalidates channel membership, selected users, ownership, and action context.

## Storage and Scheduling

Each reminder is stored under its own KV key, with a separate reminder ID index. Process-local locking protects index and reminder updates. Successful sends set `fired_at` to prevent duplicate delivery during normal polling.

The scheduler is intended for a single Mattermost node. A high-availability deployment requires a cluster mutex or compare-and-swap claim mechanism. Strict exactly-once delivery would additionally require an idempotency key or transactional outbox because a process can stop after creating a post but before persisting `fired_at`.

## Implementation Rules

- Keep source-code comments in English.
- Keep all Markdown documentation in English.
- Keep React and ReactDOM external so the bundle uses the Mattermost-provided instances.
- Check Mattermost `*model.AppError` return values explicitly to avoid typed-nil errors.
- Read HTTP response text before JSON parsing so non-JSON server errors remain diagnosable.
- Package the icon in both `assets/` for the bot profile and `public/` for the channel-header action.

