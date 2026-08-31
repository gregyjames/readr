# Design Spec: Remove localStorage and Migrate to settings.json

## 1. Overview
The current architecture relies on browser `localStorage` as a caching layer for configuration (API keys, models, agent toggles, and UI preferences). This has led to synchronization bugs between the frontend and the backend's `settings.json`. 

This design completely eliminates `localStorage`. The backend `settings.json` becomes the sole source of truth. The frontend will rely on a Vue reactive singleton to hold state, and the backend will no longer require API keys to be passed via HTTP headers.

## 2. Backend Architecture (`main.go` & `pool.go`)
- **Expanded ServerSettings Struct:** Add `Theme` (string), `ViewMode` (string), and `GraphContextExpansion` (bool) to the `ServerSettings` struct so `settings.json` can store all UI preferences natively.
- **Default Fallbacks:** Update `loadServerSettings` to provide sensible defaults for the new fields (e.g., `Theme: "light"`, `ViewMode: "card"`).
- **Remove Header Parsing:** Simplify `extractOpenRouterCredentials` in `main.go`. It will no longer inspect `X-Openrouter-Key` or `X-Openrouter-Model` headers. Instead, it will read directly from the in-memory `serverSettings`.
- **Simplify Agent Payload:** Background jobs (auto-linker, enricher) will no longer need `api_key` and `model` passed in their `Job.Payload`. 
- **Agent Credential Resolution:** Update `pool.go`'s `resolveCredentials` to strictly read from `settings.json` on disk, ignoring any payload overrides.

## 3. Frontend Architecture (`src/store/settings.ts`)
- **Reactive Singleton:** Create `src/store/settings.ts` exporting a reactive `settings` object containing all configuration and UI preference fields.
- **Initialization:** Expose an `initSettings()` async function that fetches `GET /api/settings` and populates the reactive object.
- **App Mount:** In `App.vue`, call `initSettings()` immediately on mount. A brief loading state will ensure the app doesn't render until UI preferences (like Dark Mode) are resolved.
- **No More Local Storage:** Remove all `localStorage.getItem`, `setItem`, and `removeItem` calls across the codebase.
- **No More Request Headers:** Remove all code in `fetch` requests that manually injects `Authorization` or `X-Openrouter-Key` headers.

## 4. Component Refactoring
- **SettingsView.vue:** 
  - Read initial state from the imported `settings` store instead of `localStorage`.
  - On "Save Settings", send a `POST /api/settings` payload containing all fields.
  - On a successful response, update the `settings` reactive store in memory.
- **ChatView.vue & Article.vue:** 
  - Import the `settings` store directly to read the API key, selected model, and graph expansion state.
- **Config Utilities:** Delete `src/utils/settings.ts` entirely, as getter functions are no longer needed.

## 5. Testing & Validation
- Ensure restarting the backend correctly loads `settings.json`.
- Ensure navigating between views keeps the reactive state intact.
- Verify that background agents correctly read credentials directly from `settings.json` without relying on HTTP request headers.
