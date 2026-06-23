# kubecritters-web

Vite + React 18 + TypeScript + Tailwind 3 dashboard scaffold for the
critterview Go server.

## Dev

```
npm i
npm run dev
```

Runs Vite on `http://localhost:5173` with `/api` and `/critters`
proxied to `http://localhost:8080` (your local `critterview`).

## Build

```
npm run build
```

Outputs static assets to `web/dist/`, which `critterview` serves at
`/k8s/` (matches Vite `base`).

## Tailwind tokens

Theme colors (`bg.*`, `border.*`, `text.*`, `accent`, `status.*`) and
the `font-mono` stack live in `tailwind.config.js`.
