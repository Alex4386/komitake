# Komitake web UI

Vite + React + TypeScript, styled with **shadcn/ui** (radix-nova) and **ReUI**:

- `@reui/frame`: sidebar sections (`c-frame-2` pattern)
- `@reui/badge`: status chips
- `resizable`: cockpit split (`c-resizable-9` pill handles)
- Card examples `c-card-2` / `c-card-4`: bordered header/footer cards
- Item example `c-item-5`: device list rows

```sh
npm install
npm run dev      # proxies /v1 (incl. WebSocket) → http://127.0.0.1:8080
npm run build    # writes dist/ for go:embed
```

Run `komitake web` against a live daemon, then open the UI.
