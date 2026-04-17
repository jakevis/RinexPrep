# RinexPrep Web UI

React frontend for RinexPrep — drag-and-drop UBX file upload, satellite visibility charts, interactive trim controls, and RINEX download.

## Stack

- **React 19** + TypeScript
- **Vite** for bundling and dev server
- **Tailwind CSS** for styling
- **Recharts** for satellite visibility charts
- **react-dropzone** for file upload

## Development

```bash
npm ci          # install dependencies
npm run dev     # start Vite dev server with hot reload
npm run build   # production build → dist/
npm run lint    # ESLint check
```

The dev server proxies API requests to the Go backend at `http://localhost:8080`.

## Production

The production build is embedded into the Go binary via `frontend/embed.go`. The Dockerfile handles building the frontend and copying assets automatically.
