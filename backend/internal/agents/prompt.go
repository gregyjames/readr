package agents

import (
	"encoding/json"
	"fmt"
	"strings"

	"example.com/backend/internal/repository"
)

func buildPipelineSchema(settings PipelineSettings) (map[string]jsonSchemaField, []string) {
	properties := make(map[string]jsonSchemaField)
	var required []string

	if settings.Summarizer {
		properties["summary"] = jsonSchemaField{
			Type: "string",
		}
		required = append(required, "summary")
	}

	if settings.Enricher {
		properties["frontmatter"] = jsonSchemaField{
			Type: "object",
			Properties: map[string]jsonSchemaField{
				"type": {
					Type: "string",
				},
				"description": {
					Type: "string",
				},
				"tags": {
					Type: "array",
					Items: &jsonSchemaField{
						Type: "string",
					},
				},
			},
			Required:             []string{"type", "description", "tags"},
			AdditionalProperties: false,
		}
		required = append(required, "frontmatter")
	}

	if settings.Linker {
		properties["links_to_inject"] = jsonSchemaField{
			Type: "array",
			Items: &jsonSchemaField{
				Type: "object",
				Properties: map[string]jsonSchemaField{
					"existing_article_id": {
						Type: "integer",
					},
					"exact_phrase_in_text": {
						Type: "string",
					},
				},
				Required:             []string{"existing_article_id", "exact_phrase_in_text"},
				AdditionalProperties: false,
			},
		}
		required = append(required, "links_to_inject")
	}

	return properties, required
}

func buildPipelinePrompt(settings PipelineSettings, body string, candidates []repository.ArticleRecord, existingVaultTags []string) string {
	var promptTasks []string

	if settings.Summarizer {
		promptTasks = append(promptTasks, `1. SUMMARY: Generate a concise, high-signal 2-3 sentence executive summary of the article in the "summary" field.`)
	}

	if settings.Enricher {
		var filteredVaultTags []string
		for _, t := range existingVaultTags {
			cleaned := repository.SanitizeObsidianTag(t)
			if cleaned != "" && cleaned != "moc" {
				filteredVaultTags = append(filteredVaultTags, cleaned)
			}
		}

		enricherPrompt := `2. FRONTMATTER (OKF Metadata):
- "type": A descriptive content type like "Reference Article", "News", "Documentation", "Playbook", "Tutorial", or "Essay".
- "description": A concise single sentence summarizing the core insight.
- "resource": Original source URL if known or found in text, otherwise empty string.
- "tags": 2 to 3 focused, lowercase keyword tags in kebab-case without spaces (e.g. "legal-tech", "artificial-intelligence", "enterprise-ai"). Spaces and special punctuation are strictly forbidden in Obsidian tags.
- TAG RULES:
  1. PRIMARY SUBJECT ONLY: Only tag the main subject/entity the article is actually about. Do NOT tag incidental mentions, competitor names, or passing references (e.g. if the article is about Google Gemini, tag "google" and "gemini", NOT competitor names like "anthropic" or "openai" unless it is a direct head-to-head comparison).
  2. STRUCTURE: Aim for 1 broad domain tag (e.g. "ai", "security"), 1 specific primary entity/tool tag (e.g. "google", "docker"), and optionally 1 application/sub-topic tag (e.g. "enterprise", "benchmarks").
  3. FORBIDDEN: Never output "moc" as a tag (reserved strictly for Librarian MOC hub notes).`

		if len(filteredVaultTags) > 0 {
			enricherPrompt += fmt.Sprintf(`
  4. REUSE MATCHING VAULT TAGS: When applicable, PREFER reusing matching tags from the "Relevant Vault Tags" list below to maintain taxonomy consistency (e.g. use "ai" instead of "artificial-intelligence" if "ai" is in the list). Only reuse tags that represent the PRIMARY subject of this note.

Relevant Vault Tags:
%s`, strings.Join(filteredVaultTags, ", "))
		}

		promptTasks = append(promptTasks, enricherPrompt)
	}

	if settings.Linker {
		var sb strings.Builder
		for _, a := range candidates {
			if repository.IsMOCArticle(a.Title, a.Tags) {
				continue
			}
			title := strings.TrimSpace(a.Title)
			if len(title) < 4 {
				continue
			}
			sb.WriteString(fmt.Sprintf("- ID: %d, Title: %s\n", a.ID, title))
		}
		existingVaultText := sb.String()

		if existingVaultText != "" {
			promptTasks = append(promptTasks, fmt.Sprintf(`3. SMART LINKING:
Identify connections (2 to 5 where applicable) to existing vault articles.
RULES:
1. "exact_phrase_in_text" must be a verbatim, case-sensitive substring from the article body.
2. Link to distinct relevant vault articles.

Existing Vault Articles:
%s`, existingVaultText))
		} else {
			promptTasks = append(promptTasks, `3. SMART LINKING:
(No existing vault articles available to link. Return an empty list [] for "links_to_inject").`)
		}
	}

	bodyRunes := []rune(body)
	truncatedBody := body
	if len(bodyRunes) > 10000 {
		truncatedBody = string(bodyRunes[:10000])
	}

	return fmt.Sprintf(`You are an intelligent knowledge curation pipeline. Analyze the article and perform ALL the following requested tasks.

%s

Article Content:
%s`, strings.Join(promptTasks, "\n\n"), truncatedBody)
}

func buildPipelinePayload(model string, prompt string, properties map[string]jsonSchemaField, required []string) ([]byte, error) {
	reqPayload := pipelineOpenRouterRequest{
		Model: model,
		Messages: []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": prompt,
			},
		},
		ResponseFormat: &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchemaDefinition{
				Name:   "unified_pipeline_response",
				Strict: true,
				Schema: jsonSchemaField{
					Type:                 "object",
					Properties:           properties,
					Required:             required,
					AdditionalProperties: false,
				},
			},
		},
	}

	return json.Marshal(reqPayload)
}
