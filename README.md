[![CI](https://github.com/gregyjames/readr/actions/workflows/ci.yml/badge.svg)](https://github.com/gregyjames/readr/actions/workflows/ci.yml)
![Docker Image Size (tag)](https://img.shields.io/docker/image-size/gjames8/readr/latest?label=Docker)

# Readr
A self-hosted, AI-native read-it-later app and knowledge graph engine powered by Vue, Tailwind, and Go. Articles are saved as clean markdown files on disk and tracked in a SQLite database.

## Features
* **AI-Native Chat**: Query your reading collection using OpenRouter models. Reference notes directly with `@` mentions to feed full article text into the conversation.
* **Autonomous Background Agents**:
  * **OKF Frontmatter Enricher**: Automatically extracts clean, standardized Open Knowledge Format (OKF) YAML frontmatter (`type`, `title`, `description`, `resource`, `tags`, `generated` audit metadata) when articles are ingested.
  * **Autonomous Graph Linker**: Discovers semantic connections between incoming articles and your existing vault, automatically injecting aliased `[[Wikilinks]]` and weaving nodes into the graph.
  * **On-Demand Reparsing**: Trigger or re-run background agents on any article at any time with live toast progress notifications.
  * **Configurable Worker Pipeline**: Toggle agents individually from the settings view to tailor background processing to your workflow.
* **Knowledge Graph Engine**: Global and per-article force-directed graph views mapping relationships between articles, tags, and wikilinks.
* **Real-Time Event Stream (SSE)**: Live server-sent event updates push background agent completions and graph changes straight to the browser without page refreshes.
* **1-Hop Graph Context Expansion**: Optional setting that allows the AI to automatically traverse your graph edges and include connected notes and backlinks in its context.
* **Wikilinks & Backlinks**: Bidirectional links between articles using `[[Title]]` and `[[Title|Alias]]` syntax, with edge tracking and floating hover previews.
* **Portable Markdown Storage**: All articles, OKF metadata, and chat histories are saved locally as markdown files, making your data easy to back up or sync with Obsidian and Logseq.
* **Custom Jinja Templates**: Customize how saved articles are structured with site-specific Jinja templates located in `$DATA_DIR/templates/` with automatic domain matching and rich context variables.
* **Lightweight and Fast**: Compact Go backend and Vue frontend with full dark mode support.

## Sample
<table>
  <tr>
    <td>
      <img src="https://github.com/gregyjames/readr/blob/main/samples/home.png?raw=true" width="350px"/>
    </td>
    <td>
      <img src="https://github.com/gregyjames/readr/blob/main/samples/article.png?raw=true" width="350px"/>
    </td>
    <td>
      <img src="https://github.com/gregyjames/readr/blob/main/samples/chat.png?raw=true" width="350px"/>
    </td>
    <td>
      <img src="https://github.com/gregyjames/readr/blob/main/samples/graph.png?raw=true" width="350px"/>
    </td>
  </tr>
</table>

## Docker Compose
```yaml
version: '3.8'

services:
  readr:
    image: "gjames8/readr:latest"
    ports:
      - "8080:8080"
    container_name: readr
    volumes:
      - ./data:/app/data
    restart: unless-stopped
```

## Custom Markdown Templates (Jinja)

Readr supports custom Jinja templates for controlling the formatting, frontmatter, and layout of ingested markdown articles. Place template files in `$DATA_DIR/templates/` (e.g. `./data/templates/` in Docker volume mounts).

### Domain Resolution Hierarchy
When an article is ingested from a URL (e.g., `https://gist.github.com/...`), Readr resolves templates in the following order:
1. **Explicit Template Selection**: Chosen template from ingest options (if specified).
2. **Subdomain Match**: E.g., `gist.github.com.jinja`.
3. **Parent Domain Match**: E.g., `github.com.jinja`.
4. **Default Built-in Template**: Used if no domain-specific template is found.

### Template Context Variables
The following variables are available inside Jinja templates:

| Variable | Type | Description |
| :--- | :--- | :--- |
| `title` | `string` | Title of the article or page |
| `source` / `url` | `string` | Canonical source URL |
| `domain` | `string` | Extracted hostname / domain name |
| `content` | `string` | Clean converted Markdown content of the article |
| `tags` | `[]string` | Array of extracted or assigned tags |
| `tags_str` | `string` | Comma-separated tag string |
| `cover_image` | `string` | URL of the cover or lead image |
| `saved_date` | `string` | Formatted date string (`YYYY-MM-DD HH:MM:SS`) |
| `timestamp` | `int64` | Unix epoch timestamp (seconds) |
| `author` | `string` | Article author (from metadata or OpenGraph) |
| `description` | `string` | Article summary or OpenGraph description |
| `site_name` | `string` | Publication or site name |
| `og` | `map[string]string` | Raw OpenGraph and meta properties (e.g. `og.title`, `og.image`) |

### Example Template (`github.com.jinja`)
Create `$DATA_DIR/templates/github.com.jinja`:

```jinja
---
title: {{ title }}
source: {{ source }}
tags: [{% for tag in tags %}"{{ tag }}"{% if not loop.last %}, {% endif %}{% endfor %}]
cover: {{ cover_image }}
saved: {{ saved_date }}
type: github-repo
---

# [{{ title }}]({{ source }})
{% if description %}
> {{ description }}
{% endif %}

---

{{ content }}
```

## Design choices
### Why markdown?
Saving as markdown keeps the library portable and easy to sync with tools like Obsidian.

### Why the knowledge graph?
Articles rarely exist in isolation. Connecting notes with wikilinks and visualizing them in a graph helps surface connections between concepts across long reading lists.

## License
MIT License

Copyright (c) 2024 Greg James

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
