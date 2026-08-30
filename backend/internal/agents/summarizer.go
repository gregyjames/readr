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

	"go.uber.org/zap"
)

var summaryBlockRegex = regexp.MustCompile(`(?m)^>\s*(?:💡\s*)?(?:\*\*)?(?:AI\s+)?Summary:(?:\*\*)?.*(?:\n>.*)*`)

func (p *AgentPool) processSummarizer(job Job) {
	rawAPIKey, _ := job.Payload["api_key"].(string)
	model, _ := job.Payload["model"].(string)

	apiKey := strings.TrimSpace(rawAPIKey)
	apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	apiKey = strings.TrimPrefix(apiKey, "bearer ")
	apiKey = strings.Trim(apiKey, `"'`)
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)

	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	if apiKey == "" {
		p.logger.Warn("API key not set in job payload. Agent cannot summarize.", zap.Int64("article_id", job.ArticleID))
		return
	}

	// 1. Get the article metadata from DB
	var article ArticleRecord
	if err := p.db.Table("articles").Where("id = ?", job.ArticleID).First(&article).Error; err != nil {
		p.logger.Error("Summarizer could not find article in DB", zap.Error(err))
		return
	}

	// 2. Read the markdown file
	filePath := filepath.Join(p.dataDirectory, "articles", fmt.Sprintf("%d.md", job.ArticleID))
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		p.logger.Error("Summarizer could not read file", zap.Error(err))
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

	// Guard: Only run LLM if the template rendered a summary block or frontmatter summary
	if !summaryBlockRegex.MatchString(content) && !strings.Contains(frontmatter, "summary:") {
		p.logger.Info("Skipping summarization: template does not use summary", zap.Int64("article_id", job.ArticleID))
		return
	}

	promptBody := body
	if len(promptBody) > 6000 {
		promptBody = promptBody[:6000]
	}

	// 3. Ask LLM to generate summary
	prompt := fmt.Sprintf("Generate a concise, high-signal 2-3 sentence executive summary of the following article.\n\nTitle: %s\n\nContent:\n%s\n\nReturn ONLY the summary text without any quotation marks or prefixes.", article.Title, promptBody)

	apiMsgs := []interface{}{
		map[string]string{
			"role":    "user",
			"content": prompt,
		},
	}

	reqPayload := openRouterRequest{
		Model:    model,
		Messages: apiMsgs,
	}
	bodyJSON, _ := json.Marshal(reqPayload)

	client := &http.Client{
		Timeout: 30 * time.Second,
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
	var bodyBytes []byte

	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(bodyJSON))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
		req.Header.Set("X-Title", "Readr Summarizer Agent")

		var err error
		resp, err = client.Do(req)
		if err != nil {
			p.logger.Error("Summarizer LLM request failed", zap.Error(err))
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
		p.logger.Error("Summarizer LLM request returned non-200 status", zap.Int("status", resp.StatusCode), zap.String("body", string(bodyBytes)))
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
		p.logger.Error("Summarizer failed to parse LLM response")
		return
	}

	summaryText := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	if summaryText == "" {
		return
	}

	// 4. Update the markdown content with the new summary
	newSummaryBlock := fmt.Sprintf("> 💡 **Summary:** %s", summaryText)

	var newContent string
	if summaryBlockRegex.MatchString(content) {
		newContent = summaryBlockRegex.ReplaceAllString(content, newSummaryBlock)
	} else if frontmatter != "" {
		newContent = frontmatter + "\n" + newSummaryBlock + "\n\n" + strings.TrimLeft(body, "\n")
	} else {
		newContent = newSummaryBlock + "\n\n" + content
	}

	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		p.logger.Error("Summarizer could not write file", zap.Error(err))
		return
	}

	p.logger.Info("Successfully generated summary for article", zap.Int64("article_id", job.ArticleID))
}
