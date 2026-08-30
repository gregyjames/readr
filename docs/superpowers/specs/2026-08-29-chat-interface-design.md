# Chat Interface & OpenRouter Integration Design

## Overview
Add a dedicated ChatGPT-style chat interface (`/chat`) to Readr. Users can converse with an LLM via OpenRouter, reference existing knowledge graph articles via an `@mention` system, and maintain multiple persistent chat sessions. 

Chat history is stored locally in a dual-format (JSON for state management, Markdown for data portability) and is isolated from the main knowledge graph.

## Architecture

### 1. Frontend (Vue 3)
* **Routes:** 
  * `/chat` (Dedicated full-page layout).
  * `/settings` (Settings configuration screen).
* **Settings Screen:**
  * A dedicated page where the user can input and save their `OPENROUTER_API_KEY`.
  * The key is stored locally (e.g., in `localStorage` or persisted via a backend config endpoint) so the user doesn't have to touch environment variables.
* **Chat Layout:**
  * **Sidebar:** Lists previous chat sessions. Includes a "New Chat" button and a delete icon for each session.
  * **Main Pane:** Displays the chronological message history and a fixed input area at the bottom.
* **@Mention System:**
  * A custom input listener detects the `@` character.
  * Triggers a floating dropdown menu populated with existing article titles (fetched from `/api/articles`).
  * Selecting an article removes the typed `@title` string and adds a visual "File Attachment Chip" above the input box (similar to Claude's artifact chips).
  * Chips can be removed by clicking an `x` icon.
* **Streaming UI:** Consumes Server-Sent Events (SSE) from the Go backend to render the LLM's response in real-time.

### 2. Backend (Go Fiber)
* **Endpoints:**
  * `GET /api/chats` - List all chat sessions.
  * `POST /api/chats` - Create a new chat session.
  * `DELETE /api/chats/:id` - Delete a chat session.
  * `GET /api/chats/:id` - Load chat history.
  * `POST /api/chats/:id/message` - Send a message and stream the response.
* **Dual-Storage Engine:**
  * Stored in `data/chats/`.
  * **State:** `[uuid].json` holds the structured array of messages (`role`, `content`, `attachments`). This is the absolute source of truth.
  * **Export:** `[uuid].md` is a human-readable mirror overwritten on every update. Formatted cleanly with headers for readability and portability.
* **Context Injection:** 
  * The frontend payload includes the user's message and a list of attached article IDs.
  * The backend reads the attached markdown files from disk.
  * Files are concatenated and wrapped in XML tags (`<article title="X">...</article>`) and injected into the OpenRouter system prompt.
* **OpenRouter Proxy:** 
  * The backend securely authenticates with OpenRouter using the user's configured API key (either passed from the frontend via an `Authorization: Bearer <key>` header, or loaded from backend configuration).
  * Streams the response chunks back to the Vue frontend via SSE (`Content-Type: text/event-stream`).

## Error Handling
* Missing or invalid OpenRouter API key returns a 401 error, prompting the user to visit the `/settings` screen.
* Attempting to attach a deleted or missing article logs a warning and skips the attachment.
* Network timeouts to OpenRouter gracefully terminate the SSE stream and append an error message to the chat state.
