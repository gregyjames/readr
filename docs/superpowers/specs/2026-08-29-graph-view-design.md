# Obsidian-style Graph View & Linking Design

## Purpose
To give users the ability to connect imported articles together and visualize their knowledge base as an interconnected network of notes and tags, similar to Obsidian.

## Architecture & Data Model

### Database
- Introduce an `ArticleLink` model in SQLite (via GORM).
- **Columns:** `SourceID` (int), `TargetID` (int).
- This ensures fast graph querying without needing to parse Markdown files on the fly.

### API Endpoints
- `POST /api/link`: Accepts `SourceID`, `TargetID`, and the original `SelectedText`. 
  - Updates the DB with the new link.
  - Opens the source article's markdown file, replaces the first instance of `SelectedText` with the Wikilink format: `[[Target Title|SelectedText]]` or `[[Target Title]]`.
- `GET /api/graph`: Returns the complete network.
  - Computes nodes (Articles + Tags) and edges (Article-to-Article + Article-to-Tag).

## Frontend Components

### 1. Inline Linking UI
- **Trigger:** Listen for text selection (`mouseup`/`selectionchange`) within the rendered markdown in `Article.vue`.
- **Action:** Show a floating "Link" button positioned above the selection.
- **Flow:** 
  1. User clicks "Link".
  2. A search popover appears, listing all available articles.
  3. User selects a target article.
  4. Frontend sends a request to `POST /api/link`.
  5. The UI updates locally to style the text as a link, and clicking it navigates to the target article.

### 2. Global Graph View (`Graph.vue`)
- **Location:** A dedicated full-screen route (e.g., `/graph`), accessible via the main navigation.
- **Rendering:** Uses a force-directed graph library (e.g., `vis-network` or a Vue wrapper) to map the entire database.
- **Nodes:** Both Articles and Tags are rendered as nodes, differentiated by color or shape.
- **Controls:** A UI toggle to hide/show Tag nodes to reduce visual clutter.
- **Interactivity:** Clicking an Article node routes to that article.

### 3. Local Graph View
- **Location:** A collapsible side panel (or bottom panel) inside `Article.vue`.
- **Rendering:** Reuses the graph component but restricts the dataset to the current article, its 1st-degree connections, and its tags.

## Testing & Error Handling
- Validate that the markdown string replacement doesn't break existing formatting (e.g., matching text inside an existing URL or code block).
- Handle missing target articles gracefully in the graph view.
