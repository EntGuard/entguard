# EntGuard management UI

Angular web UI for the EntGuard orchestrator. The orchestrator serves the built app from `dist/`.

## Development server

Run `npm start` and open `http://localhost:4300/`. Requests to `/api/v1` are proxied to an orchestrator on `localhost:8080` (see `src/proxy.conf.json`).

## Build

Run `npm run build`. The production build is written to `dist/`.

## Lint

Run `npm run lint`.
