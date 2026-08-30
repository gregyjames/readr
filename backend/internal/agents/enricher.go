package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

type openRouterRequest struct {
	Model          string        `json:"model"`
	Messages       []interface{} `json:"messages"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

type ArticleRecord struct {
	ID    int64
	Title string
	URL   string
}

type llmLink struct {
	ExistingArticleID int64  `json:"existing_article_id"`
	ExactPhraseInText string `json:"exact_phrase_in_text"`
}

type llmResponseJSON struct {
	YamlFrontmatter string    `json:"yaml_frontmatter"`
	LinksToInject   []llmLink `json:"links_to_inject"`
}

func (p *AgentPool) processEnrichFrontmatter(job Job) {
	apiKey, _ := job.Payload["api_key"].(string)
	model, _ := job.Payload["model"].(string)
	doEnricher, _ := job.Payload["do_enricher"].(bool)
	doLinking, _ := job.Payload["do_linking"].(bool)

	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	if apiKey == "" {
		p.logger.Warn("API key not set in job payload. Agent cannot enrich frontmatter.", zap.Int64("article_id", job.ArticleID))
		return
	}
	// 1. Get the article metadata from DB
	var article ArticleRecord
	if err := p.db.Table("articles").Where("id = ?", job.ArticleID).First(&article).Error; err != nil {
		p.logger.Error("Enricher could not find article in DB", zap.Error(err))
		return
	}

	// 1b. Get all other existing articles for linking (if enabled)
	var existingVaultText string
	var allArticles []ArticleRecord
	if doLinking {
		if err := p.db.Table("articles").Where("id != ?", job.ArticleID).Find(&allArticles).Error; err == nil {
			var sb strings.Builder
			for _, a := range allArticles {
				if len(strings.TrimSpace(a.Title)) >= 4 { // skip very short titles
					sb.WriteString(fmt.Sprintf("- ID: %d, Title: %s\n", a.ID, a.Title))
				}
			}
			existingVaultText = sb.String()
		}
	}

	// 2. Read the markdown file
	filePath := filepath.Join(p.dataDirectory, "articles", fmt.Sprintf("%d.md", job.ArticleID))
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		p.logger.Error("Enricher could not read file", zap.Error(err))
		return
	}
	content := string(contentBytes)

	// Strip existing frontmatter for the LLM to read just the body
	body := content
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) == 3 {
			body = parts[2]
		}
	}
	
	if len(body) > 10000 {
		body = body[:10000] // truncate to save tokens
	}

	// 3. Ask LLM to generate OKF frontmatter + Semantic Links
	// We force the LLM to output a JSON object.
	prompt := fmt.Sprintf(`You are an expert knowledge curator and semantic graph builder.

TASK 1 (Frontmatter): Generate ONLY a valid YAML frontmatter block (enclosed in ---) following the Open Knowledge Format (OKF) specification for the following article.
- 'type': Must be a descriptive type like 'Reference Article', 'News', 'Documentation', 'Playbook', etc.
- 'title': Create a clean, readable title.
- 'description': A single sentence summarizing the core insight.
- 'resource': Use this URL: %s
- 'tags': A YAML list of 3-5 lowercase string tags.
- 'generated': { by: agent/readr-enricher, at: %s }

TASK 2 (Semantic Links): Identify organic connections between the new article and the existing vault articles. 
Do NOT rewrite the article body. Instead, return a list of precise phrases from the text that should be converted into wikilinks.

Existing Vault Articles:
%s

RETURN FORMAT:
You must output a strictly valid JSON object matching this schema. Output ONLY JSON, with no markdown formatting around it:
{
  "yaml_frontmatter": "---...---",
  "links_to_inject": [
    {
      "existing_article_id": <int>,
      "exact_phrase_in_text": "<exact string from the article body to replace>"
    }
  ]
}

Article Content:
%s`, article.URL, time.Now().UTC().Format(time.RFC3339), existingVaultText, body)

	apiMsgs := []interface{}{
		map[string]interface{}{
			"role":    "user",
			"content": prompt,
		},
	}
	
	reqPayload := openRouterRequest{
		Model:    model,
		Messages: apiMsgs,
		ResponseFormat: &struct {
			Type string `json:"type"`
		}{Type: "json_object"},
	}

	bodyJSON, _ := json.Marshal(reqPayload)

	req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
	req.Header.Set("X-Title", "Readr Vault Agent")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		p.logger.Error("Enricher LLM request failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		p.logger.Error("Enricher LLM request returned non-200 status", zap.Int("status", resp.StatusCode), zap.String("body", string(bodyBytes)))
		return
	}

	var llmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil || len(llmResp.Choices) == 0 {
		p.logger.Error("Enricher failed to parse LLM response")
		return
	}
	
	rawJSON := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	if strings.HasPrefix(rawJSON, "```json") {
		rawJSON = strings.TrimPrefix(rawJSON, "```json")
		rawJSON = strings.TrimSuffix(rawJSON, "```")
	}

	var parsed llmResponseJSON
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		p.logger.Error("Enricher failed to parse LLM JSON", zap.Error(err), zap.String("raw", rawJSON))
		return
	}

	yamlBlock := strings.TrimSpace(parsed.YamlFrontmatter)
	if !strings.HasPrefix(yamlBlock, "---") || !strings.HasSuffix(yamlBlock, "---") {
		p.logger.Error("Enricher LLM did not return valid frontmatter")
		return
	}
	
	// 4. Inject Semantic Links using Go
	fullBody := content
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) == 3 {
			fullBody = parts[2]
		}
	}
	
	linksAdded := 0
	if doLinking && len(parsed.LinksToInject) > 0 {
		for _, link := range parsed.LinksToInject {
			// Ensure it's not empty and exists in the body
			if link.ExactPhraseInText == "" {
				continue
			}
			
			// Find the target article title
			var targetTitle string
			for _, a := range allArticles {
				if a.ID == link.ExistingArticleID {
					targetTitle = a.Title
					break
				}
			}
			if targetTitle == "" {
				continue // article ID doesn't exist
			}
			
			// Simple check if it's already linked
			alreadyLinked := fmt.Sprintf("[[%s]]", targetTitle)
			if !strings.Contains(fullBody, alreadyLinked) && strings.Contains(fullBody, link.ExactPhraseInText) {
				// Replace just one occurrence safely
				fullBody = strings.Replace(fullBody, link.ExactPhraseInText, alreadyLinked, 1)
				
				// Insert edge in SQLite
				var count int64
				p.db.Table("article_links").Where("source_id = ? AND target_id = ?", job.ArticleID, link.ExistingArticleID).Count(&count)
				if count == 0 {
					p.db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", job.ArticleID, link.ExistingArticleID)
					linksAdded++
				}
			}
		}
	}

	// 5. Overwrite the file safely
	var newContent string
	if doEnricher {
		newContent = yamlBlock + "\n\n" + strings.TrimSpace(fullBody)
	} else {
		// Just rewrite the body with the new links, preserving original frontmatter if it existed
		originalFrontmatter := ""
		if strings.HasPrefix(content, "---\n") {
			parts := strings.SplitN(content, "---\n", 3)
			if len(parts) == 3 {
				originalFrontmatter = "---\n" + parts[1] + "---\n"
			}
		}
		newContent = originalFrontmatter + strings.TrimSpace(fullBody)
	}
	
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		p.logger.Error("Enricher could not write file", zap.Error(err))
		return
	}

	if linksAdded > 0 && p.InvalidateGraphCache != nil {
		p.InvalidateGraphCache()
	}

	p.logger.Info("Successfully executed Smart Agent!", zap.Int64("article_id", job.ArticleID), zap.Int("links_added", linksAdded))
}
