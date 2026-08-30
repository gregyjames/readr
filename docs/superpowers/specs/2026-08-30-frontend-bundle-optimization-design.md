# Frontend Bundle Optimization & Code Splitting Design

## 1. Overview
Optimize Readr's frontend build output by eliminating the monolithic 1.7 MB JavaScript bundle. Implement dynamic route-level lazy loading in Vue Router and configure granular vendor chunk splitting in Vite to achieve sub-second initial page loads and long-term browser cacheability.

---

## 2. Problem Statement
The current build bundles all route components and third-party dependencies (`vis-network`, `highlight.js`, `marked`, `vue`, `axios`) into a single `index-*.js` file exceeding 1.7 MB minified:
* Users downloading the homepage (`/`) are forced to download the entire `vis-network` graph visualization library (~1.1 MB) and code highlighting engines upfront.
* Vite emits warning: `(!) Some chunks are larger than 500 kB after minification.`

---

## 3. Architecture & Design

### 3.1 Dynamic Route Lazy Loading (`frontend/src/router.ts`)
Convert all synchronous page imports to dynamic imports with `() => import(...)`:
* `/` -> `Home.vue`
* `/articles/:id` -> `Article.vue`
* `/graph` -> `Graph.vue`
* `/chat` & `/chat/:id` -> `ChatView.vue`
* `/settings` -> `SettingsView.vue`

### 3.2 Vendor Chunk Splitting (`frontend/vite.config.ts`)
Configure Vite build options (`build.rollupOptions.output.manualChunks`):
* `vendor-graph`: `['vis-network']`
* `vendor-markdown`: `['marked', 'highlight.js']`
* `vendor-core`: `['vue', 'vue-router', 'axios', 'mitt']`

---

## 4. Expected Performance Metrics
* **Initial Page Load Bundle:** Dropping from ~1.72 MB to ~80 KB (~95% reduction).
* **Network Efficiency:** Graph physics engine loaded on-demand.
* **Build Cleanliness:** Zero chunk size warnings during `bun run build` and Docker container builds.

---

## 5. Verification Plan
1. **TypeScript & Tests:** Run `bun test` and `bun run build` to verify clean compilation without chunk warnings.
2. **Chunk Inspection:** Verify that `dist/assets/` outputs separate chunks for vendor libraries and views.
3. **End-to-End Navigation:** Verify seamless client-side navigation between Home, Article, Graph, Chat, and Settings.
