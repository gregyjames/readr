# Readr Domain Glossary & Architecture Concepts

This document establishes the canonical domain language and architecture vocabulary for the Readr codebase.

---

## Domain Model

- **Vault**: The primary domain boundary managing markdown notes, physical directories, assets, and their synchronization with SQLite database records and full-text search indexes.
- **Article / Note**: A captured, saved, or imported web article or note stored as a Markdown file in the vault with frontmatter metadata.
- **Topic Folder**: A subfolder within `data/articles/<Topic>/` containing related articles grouped by subject.
- **Map of Content (MOC)**: An overview document (e.g. `MOC - Lifehacks.md`) synthesized by the Librarian agent that categorizes, connects, and summarizes notes within a topic.
- **MOC Document**: The markdown AST representation and wikilink reconciliation engine for Map of Content hub notes.
- **Cluster Classifier**: The pure domain classifier that groups vault notes into topic clusters, evaluates topic specificity, and filters out competitor notes.
- **Article Link**: A directional link or relation between two articles (e.g. via Obsidian-style `[[wikilinks]]` or relational entries in `article_links`).
- **Attachment / Asset**: Media files (images, audio) associated with an article, stored in `data/images/<id>/`.
- **Librarian**: The background AI agent responsible for clustering notes, synthesizing MOCs, and pruning stale or orphan topic structures.

---

## Architecture Vocabulary & Principles

- **Module**: A distinct unit of code (package, struct, interface) that hides implementation details behind a clean boundary.
- **Interface**: The public surface through which callers interact with a module. "The interface is the test surface."
- **Depth**: A measure of leverage — deep modules provide powerful functionality through a small, simple interface. Shallow modules provide thin abstractions where the interface is nearly as complex as the implementation.
- **Seam**: A clear dividing line between components that allows independent modification, substitution, or testing.
- **Adapter**: A thin translation layer connecting an external interface (HTTP route, cron schedule, CLI) to an internal domain module.
- **Leverage**: The ratio of internal work/invariants handled by a module relative to the cognitive burden required of callers.
- **Locality**: Placing related state, operations, and invariants close together so reasoning about a concept does not require jumping across distant subsystems.
- **Deletion Test**: If deleting a module concentrates complexity into a cleaner abstraction rather than just scattering it, it should be deepened or eliminated.
