package agents

import (
	"bytes"
	"context"
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
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type OKFFrontmatterResponse struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Resource    string   `json:"resource"`
	Tags        []string `json:"tags"`
}

type UnifiedPipelineResponse struct {
	Summary       string                  `json:"summary,omitempty"`
	Frontmatter   *OKFFrontmatterResponse `json:"frontmatter,omitempty"`
	LinksToInject []llmLink               `json:"links_to_inject,omitempty"`
}

type pipelineOpenRouterRequest struct {
	Model          string          `json:"model"`
	Messages       []interface{}   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

func injectLinksIntoBody(body string, links []llmLink, candidates []repository.ArticleRecord, sourceID int64, db *gorm.DB) string {
	if len(links) == 0 {
		return body
	}

	reProtected := regexp.MustCompile(`(\[\[[\s\S]*?\]\]|\[[\s\S]*?\]\([\s\S]*?\)|` + "`[\\s\\S]*?`" + `)`)

	for _, link := range links {
		phrase := strings.TrimSpace(link.ExactPhraseInText)
		if phrase == "" {
			continue
		}

		var targetTitle string
		for _, a := range candidates {
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
			if db != nil {
				var count int64
				db.Table("article_links").Where("source_id = ? AND target_id = ?", sourceID, link.ExistingArticleID).Count(&count)
				if count == 0 {
					db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", sourceID, link.ExistingArticleID)
				}
			}
		}
	}
	return body
}

func (p *AgentPool) processPipeline(job Job) {
	p.processPipelineWithURL(job, "https://openrouter.ai/api/v1/chat/completions")
}

func (p *AgentPool) processPipelineWithURL(job Job, apiURL string) {
	if !job.Settings.Summarizer && !job.Settings.Enricher && !job.Settings.Linker {
		p.logger.Info("Pipeline skipped: no stages enabled", zap.Int64("article_id", job.ArticleID))
		return
	}

	apiKey, model := p.resolveCredentials(job)
	if apiKey == "" {
		p.logger.Warn("API key not configured. Agent cannot run pipeline.", zap.Int64("article_id", job.ArticleID))
		return
	}

	if p.repo == nil && p.db != nil {
		p.repo = repository.NewGormRepository(p.db)
	}

	// 1. Read article metadata from repo or db
	var articleTitle string
	if p.repo != nil {
		if a, err := p.repo.FindByID(context.Background(), job.ArticleID); err == nil && a != nil {
			articleTitle = a.Title
		}
	}
	if articleTitle == "" && p.db != nil {
		var a ArticleRecord
		if err := p.db.Table("articles").Where("id = ?", job.ArticleID).First(&a).Error; err == nil {
			articleTitle = a.Title
		}
	}

	// 2. Read markdown file
	filePath := filepath.Join(p.dataDirectory, "articles", fmt.Sprintf("%d.md", job.ArticleID))
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		p.logger.Error("Pipeline could not read file", zap.Error(err), zap.Int64("article_id", job.ArticleID))
		return
	}
	content := string(contentBytes)

	// Separate frontmatter and body
	frontmatter := ""
	body := content
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) == 3 {
			frontmatter = "---\n" + parts[1] + "---\n"
			body = parts[2]
		}
	}

	// 3. Candidates for Linker
	var candidates []repository.ArticleRecord
	var existingVaultText string
	if job.Settings.Linker && p.repo != nil {
		candList, err := p.repo.FindCandidates(context.Background(), job.ArticleID, articleTitle, body, 15)
		if err != nil {
			p.logger.Error("Pipeline could not find candidates", zap.Error(err))
		} else {
			candidates = candList
			var sb strings.Builder
			for _, a := range candidates {
				title := strings.TrimSpace(a.Title)
				if len(title) < 4 {
					continue
				}
				sb.WriteString(fmt.Sprintf("- ID: %d, Title: %s\n", a.ID, title))
			}
			existingVaultText = sb.String()
		}
	}

	// 4. Assemble dynamic json_schema and prompt tasks
	properties := make(map[string]jsonSchemaField)
	var required []string
	var promptTasks []string

	if job.Settings.Summarizer {
		properties["summary"] = jsonSchemaField{
			Type: "string",
		}
		required = append(required, "summary")
		promptTasks = append(promptTasks, `1. SUMMARY: Generate a concise, high-signal 2-3 sentence executive summary of the article in the "summary" field.`)
	}

	if job.Settings.Enricher {
		properties["frontmatter"] = jsonSchemaField{
			Type: "object",
			Properties: map[string]jsonSchemaField{
				"type": {
					Type: "string",
				},
				"title": {
					Type: "string",
				},
				"description": {
					Type: "string",
				},
				"resource": {
					Type: "string",
				},
				"tags": {
					Type: "array",
					Items: &jsonSchemaField{
						Type: "string",
					},
				},
			},
			Required:             []string{"type", "title", "description", "resource", "tags"},
			AdditionalProperties: false,
		}
		required = append(required, "frontmatter")
		promptTasks = append(promptTasks, `2. FRONTMATTER (OKF Metadata):
- "type": A descriptive content type like "Reference Article", "News", "Documentation", "Playbook", "Tutorial", or "Essay".
- "title": Clean, readable, informative title.
- "description": A concise single sentence summarizing the core insight.
- "resource": Original source URL if known or found in text, otherwise empty string.
- "tags": 3 to 5 lowercase keyword tags.`)
	}

	if job.Settings.Linker {
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

		vaultSection := existingVaultText
		if vaultSection == "" {
			vaultSection = "(No existing vault articles available to link)"
		}

		promptTasks = append(promptTasks, fmt.Sprintf(`3. SMART LINKING:
Analyze the article content and identify relevant connections (aim for 2 to 5 connections where applicable) to existing articles in the user's vault.
Look for mentions of shared companies, organizations, people, shared technologies/tools/languages, or overlapping concepts/themes.
RULES for links_to_inject:
- "exact_phrase_in_text" must be a verbatim, case-sensitive substring from the article body.
- "existing_article_id" must match the ID from Existing Vault Articles list.
- If no matches exist or vault articles list is empty, return an empty array [].

Existing Vault Articles:
%s`, vaultSection))
	}

	bodyRunes := []rune(body)
	truncatedBody := body
	if len(bodyRunes) > 10000 {
		truncatedBody = string(bodyRunes[:10000])
	}

	prompt := fmt.Sprintf(`You are an expert knowledge curator and knowledge graph builder.
Perform the following tasks for the article content provided below:

%s

Article Title: %s

Article Content:
%s`, strings.Join(promptTasks, "\n\n"), articleTitle, truncatedBody)

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

	bodyJSON, err := json.Marshal(reqPayload)
	if err != nil {
		p.logger.Error("Pipeline failed to marshal request payload", zap.Error(err))
		return
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if len(via) > 0 {
				req.Header.Set("Authorization", via[0].Header.Get("Authorization"))
			}
			return nil
		},
	}

	var resp *http.Response
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyJSON))
		if err != nil {
			p.logger.Error("Pipeline failed to create HTTP request", zap.Error(err))
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
		req.Header.Set("X-Title", "Readr Pipeline Agent")

		resp, err = client.Do(req)
		if err != nil {
			p.logger.Error("Pipeline LLM request failed", zap.Error(err))
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
		bodyBytes, _ := io.ReadAll(resp.Body)
		p.logger.Error("Pipeline LLM request returned non-200 status", zap.Int("status", resp.StatusCode), zap.String("body", string(bodyBytes)))
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
		p.logger.Error("Pipeline failed to parse LLM response")
		return
	}

	rawJSON := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	if strings.HasPrefix(rawJSON, "```json") {
		rawJSON = strings.TrimPrefix(rawJSON, "```json")
		rawJSON = strings.TrimSuffix(rawJSON, "```")
	}

	var pipelineResp UnifiedPipelineResponse
	if err := json.Unmarshal([]byte(rawJSON), &pipelineResp); err != nil {
		p.logger.Error("Pipeline failed to unmarshal JSON into UnifiedPipelineResponse", zap.Error(err), zap.String("raw", rawJSON))
		return
	}

	// In-memory transformations
	// 1. Summarizer
	if job.Settings.Summarizer && strings.TrimSpace(pipelineResp.Summary) != "" {
		summaryText := strings.TrimSpace(pipelineResp.Summary)
		newSummaryBlock := fmt.Sprintf("> 💡 **Summary:** %s", summaryText)
		if summaryBlockRegex.MatchString(body) {
			body = summaryBlockRegex.ReplaceAllString(body, newSummaryBlock)
		} else {
			body = newSummaryBlock + "\n\n" + strings.TrimLeft(body, "\n")
		}
	}

	// 2. Linker
	if job.Settings.Linker && len(pipelineResp.LinksToInject) > 0 {
		body = injectLinksIntoBody(body, pipelineResp.LinksToInject, candidates, job.ArticleID, p.db)
	}

	// 3. Enricher Frontmatter
	if job.Settings.Enricher && pipelineResp.Frontmatter != nil {
		tags := make([]string, len(pipelineResp.Frontmatter.Tags))
		for i, tag := range pipelineResp.Frontmatter.Tags {
			tags[i] = strings.ToLower(strings.TrimSpace(tag))
		}
		metadata := OKFMetadata{
			Type:        pipelineResp.Frontmatter.Type,
			Title:       pipelineResp.Frontmatter.Title,
			Description: pipelineResp.Frontmatter.Description,
			Resource:    pipelineResp.Frontmatter.Resource,
			Tags:        tags,
			Generated: OKFGeneratedInfo{
				By: "agent/readr-pipeline",
				At: time.Now().UTC().Format(time.RFC3339),
			},
		}
		yamlBytes, err := yaml.Marshal(&metadata)
		if err != nil {
			p.logger.Error("Pipeline failed to serialize YAML frontmatter", zap.Error(err))
			return
		}
		frontmatter = "---\n" + string(yamlBytes) + "---\n\n"
	}

	// Build consolidated content
	var newContent string
	if frontmatter != "" {
		newContent = strings.TrimRight(frontmatter, "\n") + "\n\n" + strings.TrimSpace(body) + "\n"
	} else {
		newContent = strings.TrimSpace(body) + "\n"
	}

	// Write consolidated file in 1 atomic write (write to tmp file then rename)
	dir := filepath.Dir(filePath)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%d.md.tmp", job.ArticleID))
	if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
		p.logger.Error("Pipeline failed to write tmp markdown file", zap.Error(err))
		return
	}
	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			p.logger.Error("Pipeline failed to write markdown file", zap.Error(err))
			return
		}
	}

	p.logger.Info("Successfully completed unified pipeline execution!", zap.Int64("article_id", job.ArticleID))
}
