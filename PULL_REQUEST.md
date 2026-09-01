# PR: Autonomous Librarian Agent, Map of Content (MOC) Synthesizer & Graph Galaxy Clustering

## 📌 Summary
This pull request introduces the **Autonomous Librarian Agent & Map of Content (MOC) Synthesizer**, a comprehensive knowledge organization system that clusters dense topic areas into structured hub notes. It includes an **85–95% token-optimized delta architecture**, **amber hexagon galaxy graph styling with tuned organic physics**, and **end-to-end pipeline telemetry** in the Settings Diagnostics dashboard.

---

## 🚀 Key Features & Changes

### 1. Autonomous Librarian Agent & MOC Synthesizer
- **Topological Topic Clustering**: Scans the vault to identify article groups sharing primary tags and interlinks that meet or exceed the configurable cluster threshold (`min_cluster_size`, default: 5 notes).
- **Structured Synthesis (`moc_synthesis`)**: Generates structured MOC hub notes (`MOC - <Topic>.md`) containing executive summaries, categorized thematic sections, and 1-sentence contextual notes for each linked article.
- **User Content Preservation**: Automatically detects and preserves all custom user writing under `## Notes & Synthesis` across updates.
- **Background Cron Scheduler & Manual Trigger**: Runs autonomously on a configurable cron schedule (e.g. `0 0 * * *` for daily 12 AM) or via 1-click on-demand execution.

### 2. Delta-Only Fast-Path & Zero-Token Skip Architecture (Token Optimization)
- **Zero-Token Skip**: Parses existing markdown wikilinks (`[[Title]]`) on disk. If an MOC cluster is already up-to-date, the LLM call is **skipped completely (0 tokens consumed, 0ms latency)**.
- **Lightweight Delta Classification (`moc_delta_placement`)**: When new notes are added to an existing MOC, the prompt sends only the existing section headers and the new unlinked notes, reducing token usage from ~3,500 down to ~150–250 tokens per update (**~95% cost reduction**).
- **Atomic Markdown File Updates**: Delta placements are appended to their target sections before user content, with atomic `.tmp` file writing.

### 3. Graph Visual System & Force Physics Overhaul
- **MOC Visual Hierarchy**: Classified under `GroupMOC` in backend topology and rendered in frontend canvas as radiant **Amber Hexagons** with glowing border halos.
- **Radial Galaxy Clustering**:
  - MOC hubs are seeded into distinct radial quadrants ($R = 480\text{px}$) with heavy anchor mass (`mass: 8.0`).
  - High-tension orbital springs (`length: 75px`, `springConstant: 0.045`) keep member notes tightly bound around their parent MOC, while cross-topic edges span loosely across empty space.
- **Organic Physics Tuning**:
  - Repulsion solver with `damping: 0.45` and `springConstant: 0.03` provides fluid, organic movement without rigidity or infinite oscillation.
  - Straight hardware-accelerated edges (`smooth: false`) and selective label display ($\ge 5$ connections) eliminate render lag.
  - Dedicated button-only snappy zoom (`zoomView: false`, 120ms transitions).

### 4. Telemetry & Diagnostics Dashboard
- **Pipeline Metrics Tracking**: Captures exact OpenRouter `prompt_tokens`, `completion_tokens`, duration, and status for every Librarian run in SQLite `pipeline_metrics`.
- **Live Diagnostics Card**: Displays Librarian schedule, next run time, clusters detected, MOCs created/updated, and execution time in the Settings Diagnostics tab.
- **History Table Integration**: Librarian runs appear in the Pipeline Execution History with a dedicated **Amber `Librarian` badge**.

---

## 🛠️ Files Modified & Created

### Backend (`backend/`)
- `internal/agents/librarian.go`: Core clustering engine, delta extraction, full & delta OpenRouter LLM schemas, cron scheduler, and atomic markdown file management.
- `internal/agents/librarian_test.go`: Unit tests for cluster detection, zero-token skip, delta prompt filtering, telemetry recording, and thread safety.
- `internal/graph/topology.go`: Group classification for `GroupMOC` and hexagon node metadata.
- `internal/handlers/librarian.go` & `diagnostics.go`: REST endpoints for `/api/librarian/status`, `/api/librarian/run`, and diagnostics telemetry.
- `internal/handlers/settings.go`: Persistence for `librarian_enabled`, `librarian_cron`, and `librarian_min_cluster_size`.
- `main.go`: Initialization and lifecycle binding for Librarian runner and cron manager.

### Frontend (`frontend/`)
- `src/components/Graph.vue`: Full knowledge graph with MOC radial clustering, amber hexagon styling, straight edges, and button zoom.
- `src/components/LocalGraph.vue`: Article-level local subgraph with MOC styling and repulsion physics.
- `src/views/SettingsView.vue`: Librarian configuration card, manual trigger button, and Diagnostics dashboard telemetry card.

---

## 🧪 Verification & Testing
- **Backend Tests**: 100% pass rate (`go test -v -count=1 ./...`).
- **Frontend Build**: Production build clean (`vue-tsc -b && vite build` built in < 2s).
- **Test Scenarios Covered**:
  - `TestLibrarian_DetectClusters`: Groups $\ge 5$ articles into topic clusters.
  - `TestLibrarian_ZeroTokenSkip_WhenUpToDate`: 0 HTTP requests when MOC is current.
  - `TestLibrarian_DeltaClassification_OnlySendsNewNotes`: Delta prompt sends only unlinked notes and existing sections.
  - `TestLibrarian_PreserveUserNotesSection`: Custom user notes retained verbatim.
  - `TestLibrarian_RecordsPipelineMetrics`: Tokens, duration, and status persisted in SQLite.
  - `TestLibrarian_ThreadSafeExecution`: Prevents concurrent execution races.
