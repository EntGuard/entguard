# EntGuard management UI

Angular web UI for the EntGuard orchestrator. The orchestrator serves the built app from `dist/`.

## Requirements

Node.js 22.22.3 or later, or 24.15 or later (required by Angular 22). Run `npm ci` to install the dependencies.

## Development server

Run `npm start` and open `http://localhost:4300/`. Requests to `/api/v1` are proxied to an orchestrator on `localhost:8080` (see `src/proxy.conf.json`).

## Build

Run `npm run build`. The production build is written to `dist/`.

## Lint

Run `npm run lint`.

## Dependency overrides

`package.json` overrides `minimatch` to `^11.0.4` to clear GHSA-g3hv-75wg-m249 (minimatch below 11.0.1). ESLint, typescript-eslint and angular-eslint still declare minimatch `^10`.

TODO: remove the override once those packages depend on minimatch 11. Check with `npm ls minimatch` after removing it.
