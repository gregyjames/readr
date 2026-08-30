# Implementation Plan: Frontend Bundle Optimization & Code Splitting

**Goal:** Implement dynamic route-level code splitting and granular vendor chunking in Vite to eliminate the 1.7 MB monolithic bundle and Vite 500 kB chunk warning.

---

## Tasks

### Task 1: Update Vue Router with Dynamic Lazy Loading
- **File:** `frontend/src/router.ts`
- Replace synchronous `import ... from ...` with `() => import(...)` for all routes.

### Task 2: Configure Vite Vendor Manual Chunks
- **File:** `frontend/vite.config.ts`
- Add `build.rollupOptions.output.manualChunks` splitting `vis-network`, `marked`/`highlight.js`, and `vue`/`vue-router`/`axios`.

### Task 3: Build & Verification
- Run `bun test` and `bun run build`.
- Verify chunk sizes in build output.
