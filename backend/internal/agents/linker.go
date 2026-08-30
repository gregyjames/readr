package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
)

type llmLink struct {
	ExistingArticleID int64  `json:"existing_article_id"`
	ExactPhraseInText string `json:"exact_phrase_in_text"`
}

type llmLinkerResponseJSON struct {
	LinksToInject []llmLink `json:"links_to_inject"`
}

type linkerOpenRouterRequest struct {
	Model          string        `json:"model"`
	Messages       []interface{} `json:"messages"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

func (p *AgentPool) processAutoLinker(job Job) {
	apiKey, _ := job.Payload["api_key"].(string)
	model, _ := job.Payload["model"].(string)

	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	if apiKey == "" {
		p.logger.Warn("API key not set in job payload. Agent cannot run auto linker.", zap.Int64("article_id", job.ArticleID))
		return
	}

	// 1. Fetch all non-deleted articles in a single query
	var allArticles []repository.ArticleRecord
	if err := p.db.Table("articles").Where("deleted_at IS NULL").Find(&allArticles).Error; err != nil {
		p.logger.Error("AutoLinker could not load articles", zap.Error(err))
		return
	}

	// 2. Build a deduplicated prompt using a HashSet
	seenTitles := make(map[string]struct{})
	var currentTitle string

	// Find the current article's title to exclude it
	for _, a := range allArticles {
		if a.ID == job.ArticleID {
			currentTitle = strings.TrimSpace(a.Title)
			seenTitles[currentTitle] = struct{}{} // Add to hashset to ignore it
			break
		}
	}

	var sb strings.Builder
	for _, a := range allArticles {
		title := strings.TrimSpace(a.Title)
		if len(title) < 4 { // skip very short titles
			continue
		}
		if _, exists := seenTitles[title]; !exists {
			sb.WriteString(fmt.Sprintf("- ID: %d, Title: %s\n", a.ID, title))
			seenTitles[title] = struct{}{} // Mark as seen
		}
	}
	
	existingVaultText := sb.String()
	if existingVaultText == "" {
		p.logger.Info("No other articles in vault to link to", zap.Int64("article_id", job.ArticleID))
		return
	}

	// 3. Read the markdown file
	filePath := filepath.Join(p.dataDirectory, "articles", fmt.Sprintf("%d.md", job.ArticleID))
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		p.logger.Error("AutoLinker could not read file", zap.Error(err))
		return
	}
	content := string(contentBytes)

	// Keep frontmatter separated so we only link in the body
	frontmatter := ""
	body := content
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) == 3 {
			frontmatter = "---\n" + parts[1] + "---\n"
			body = parts[2]
		}
	}
	
	truncatedBody := body
	if len(truncatedBody) > 10000 {
		truncatedBody = truncatedBody[:10000] // truncate for LLM input
	}

	// 4. Ask LLM to generate Semantic Links
	prompt := fmt.Sprintf(`You are an expert semantic knowledge graph builder.
Analyze the article content and identify ALL relevant connections (aim for 2 to 5 connections where applicable) to existing articles in the user's vault.

Look for:
- Mentions of shared companies, organizations, or people (e.g. "Anthropic", "OpenAI", "Google")
- Shared technologies, languages, tools, or frameworks (e.g. "Python", "Mojo", "VPN", "LLM")
- Overlapping concepts, themes, or subject matter (e.g. "AI safety", "paywalls", "security", "copyright")

RULES:
1. "exact_phrase_in_text" must be a verbatim, case-sensitive substring from the article body.
2. The phrase can be a short mention, entity name, or concept (e.g. "Anthropic" or "AI models") that relates to the vault article.
3. Link to as many distinct relevant vault articles as make sense (do not limit yourself to just 1 link if more connections exist).

Existing Vault Articles:
%s

RETURN FORMAT:
You must output a strictly valid JSON object matching this schema. Output ONLY JSON, with no markdown formatting around it:
{
  "links_to_inject": [
    {
      "existing_article_id": <int>,
      "exact_phrase_in_text": "<exact verbatim phrase from article body>"
    }
  ]
}

Article Content:
%s`, existingVaultText, truncatedBody)

	apiMsgs := []interface{}{
		map[string]interface{}{
			"role":    "user",
			"content": prompt,
		},
	}
	
	reqPayload := linkerOpenRouterRequest{
		Model:     model,
		Messages:  apiMsgs,
		MaxTokens: 2500, // Generous token ceiling to allow reasoning models room to think before outputting JSON
		ResponseFormat: &struct {
			Type string `json:"type"`
		}{Type: "json_object"},
	}

	bodyJSON, _ := json.Marshal(reqPayload)

	client := &http.Client{Timeout: 60 * time.Second}
	var resp *http.Response
	var bodyBytes []byte

	// Try up to 2 times with backoff on in-flight budget limits
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(bodyJSON))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
		req.Header.Set("X-Title", "Readr Vault Agent")

		var err error
		resp, err = client.Do(req)
		if err != nil {
			p.logger.Error("AutoLinker LLM request failed", zap.Error(err))
			return
		}

		if resp.StatusCode == http.StatusPaymentRequired || resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			time.Sleep(3 * time.Second)
			continue
		}
		break
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ = io.ReadAll(resp.Body)
		p.logger.Error("AutoLinker LLM request returned non-200 status", zap.Int("status", resp.StatusCode), zap.String("body", string(bodyBytes)))
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
		p.logger.Error("AutoLinker failed to parse LLM response")
		return
	}
	
	rawJSON := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	if strings.HasPrefix(rawJSON, "```json") {
		rawJSON = strings.TrimPrefix(rawJSON, "```json")
		rawJSON = strings.TrimSuffix(rawJSON, "```")
	}

	var parsed llmLinkerResponseJSON
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		p.logger.Error("AutoLinker failed to parse LLM JSON", zap.Error(err), zap.String("raw", rawJSON))
		return
	}

	// 5. Inject Semantic Links using Go with Protected Boundary Replacement
	linksAdded := 0
	if len(parsed.LinksToInject) > 0 {
		// Regex matching markdown links, existing wikilinks, and code blocks
		reProtected := regexp.MustCompile(`(\[\[[\s\S]*?\]\]|\[[\s\S]*?\]\([\s\S]*?\)|` + "`[\\s\\S]*?`" + `)`)

		for _, link := range parsed.LinksToInject {
			phrase := strings.TrimSpace(link.ExactPhraseInText)
			if phrase == "" {
				continue
			}
			
			var targetTitle string
			for _, a := range allArticles {
				if a.ID == link.ExistingArticleID {
					targetTitle = a.Title
					break
				}
			}
			if targetTitle == "" {
				continue
			}
			
			// Format wikilink (aliased if phrase is different from title)
			var replacement string
			if strings.EqualFold(phrase, targetTitle) {
				replacement = fmt.Sprintf("[[%s]]", targetTitle)
			} else {
				replacement = fmt.Sprintf("[[%s|%s]]", targetTitle, phrase)
			}

			// Don't inject if already linked to this target anywhere in body
			alreadyLinkedSimple := fmt.Sprintf("[[%s]]", targetTitle)
			alreadyLinkedAliasedPrefix := fmt.Sprintf("[[%s|", targetTitle)
			if strings.Contains(body, alreadyLinkedSimple) || strings.Contains(body, alreadyLinkedAliasedPrefix) {
				continue
			}

			// Split into protected tokens and unprotected segments
			parts := reProtected.Split(body, -1)
			matches := reProtected.FindAllString(body, -1)

			replaced := false
			var newBody strings.Builder
			for i, part := range parts {
				if !replaced && strings.Contains(part, phrase) {
					part = strings.Replace(part, phrase, replacement, 1)
					replaced = true
				}
				newBody.WriteString(part)
				if i < len(matches) {
					newBody.WriteString(matches[i])
				}
			}

			if replaced {
				body = newBody.String()
				var count int64
				p.db.Table("article_links").Where("source_id = ? AND target_id = ?", job.ArticleID, link.ExistingArticleID).Count(&count)
				if count == 0 {
					p.db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", job.ArticleID, link.ExistingArticleID)
					linksAdded++
				}
			}
		}
	}

	// 6. Overwrite the file safely
	if linksAdded > 0 {
		newContent := frontmatter + body
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			p.logger.Error("AutoLinker could not write file", zap.Error(err))
			return
		}
		p.logger.Info("Successfully executed Smart Linker!", zap.Int64("article_id", job.ArticleID), zap.Int("links_added", linksAdded))
	} else {
		p.logger.Info("Smart Linker found no new connections", zap.Int64("article_id", job.ArticleID))
	}
}
