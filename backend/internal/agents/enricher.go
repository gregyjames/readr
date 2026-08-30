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
	Model     string        `json:"model"`
	Messages  []interface{} `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type ArticleRecord struct {
	ID    int64
	Title string
}

func (p *AgentPool) processEnrichFrontmatter(job Job) {
	apiKey, _ := job.Payload["api_key"].(string)
	model, _ := job.Payload["model"].(string)
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
	
	if len(body) > 8000 {
		body = body[:8000] // truncate to save tokens
	}

	// 3. Ask LLM to generate OKF frontmatter
	prompt := fmt.Sprintf(`You are an expert knowledge curator. Generate ONLY a valid YAML frontmatter block (enclosed in ---) following the Open Knowledge Format (OKF) specification for the following article.
Requirements:
- 'type': Must be a descriptive type like 'Reference Article', 'News', 'Documentation', 'Playbook', etc.
- 'title': Create a clean, readable title.
- 'description': A single sentence summarizing the core insight.
- 'resource': Original source URL if known.
- 'tags': A YAML list of 3-5 lowercase string tags.
- 'generated': { by: agent/readr-enricher, at: %s }

Article Content:
%s

Output ONLY the YAML block starting and ending with ---. No other text.`, time.Now().UTC().Format(time.RFC3339), body)

	apiMsgs := []interface{}{
		map[string]string{
			"role":    "user",
			"content": prompt,
		},
	}

	reqPayload := openRouterRequest{
		Model:     model,
		Messages:  apiMsgs,
		MaxTokens: 2500, // Generous token ceiling to allow reasoning models room to think before outputting YAML
	}
	bodyJSON, _ := json.Marshal(reqPayload)

	client := &http.Client{Timeout: 30 * time.Second}
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
			p.logger.Error("Enricher LLM request failed", zap.Error(err))
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

	yamlBlock := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	if !strings.HasPrefix(yamlBlock, "---") {
		// Try to extract if it included markdown code fences
		if strings.Contains(yamlBlock, "```yaml\n---") {
			yamlBlock = strings.Split(yamlBlock, "```yaml\n")[1]
			yamlBlock = strings.Split(yamlBlock, "```")[0]
		}
	}
	
	yamlBlock = strings.TrimSpace(yamlBlock)
	if !strings.HasPrefix(yamlBlock, "---") || !strings.HasSuffix(yamlBlock, "---") {
		p.logger.Error("Enricher LLM did not return valid frontmatter")
		return
	}

	// 4. Overwrite the file with the new pristine OKF frontmatter
	newContent := yamlBlock + "\n\n" + strings.TrimSpace(body)
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		p.logger.Error("Enricher could not write file", zap.Error(err))
		return
	}

	p.logger.Info("Successfully enriched article to OKF format!", zap.Int64("article_id", job.ArticleID))
}
