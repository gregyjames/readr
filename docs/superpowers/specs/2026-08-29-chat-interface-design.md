# Chat Interface & OpenRouter Integration Design

## Overview
Add a dedicated ChatGPT-style chat interface (`/chat`) to Readr. Users can converse with an LLM via OpenRouter, reference existing knowledge graph articles via an `@mention` system, and maintain multiple persistent chat sessions. 

Chat history is stored locally in a dual-format (JSON for state management, Markdown for data portability) and is isolated from the main knowledge graph.

## Architecture

### 1. Frontend (Vue 3)
* **Route:** `/chat` (Dedicated full-page layout).
* **Layout:**
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
  * The backend securely authenticates with OpenRouter using an `OPENROUTER_API_KEY` environment variable.
  * Streams the response chunks back to the Vue frontend via SSE (`Content-Type: text/event-stream`).

## Error Handling
* Missing `OPENROUTER_API_KEY` returns a 500 error prompting the user to configure their environment.
* Attempting to attach a deleted or missing article logs a warning and skips the attachment.
* Network timeouts to OpenRouter gracefully terminate the SSE stream and append an error message to the chat state.
