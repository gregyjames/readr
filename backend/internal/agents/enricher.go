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
	"gopkg.in/yaml.v3"
)

type OKFMetadata struct {
	Type        string           `json:"type" yaml:"type"`
	Title       string           `json:"title" yaml:"title"`
	Description string           `json:"description" yaml:"description"`
	Resource    string           `json:"resource,omitempty" yaml:"resource,omitempty"`
	Tags        []string         `json:"tags" yaml:"tags"`
	Generated   OKFGeneratedInfo `json:"-" yaml:"generated"`
}

type OKFGeneratedInfo struct {
	By string `yaml:"by"`
	At string `yaml:"at"`
}

type enricherOpenRouterRequest struct {
	Model          string          `json:"model"`
	Messages       []interface{}   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type ArticleRecord struct {
	ID    int64
	Title string
}

func (p *AgentPool) processEnrichFrontmatter(job Job) {
	p.processEnrichFrontmatterWithURL(job, "https://openrouter.ai/api/v1/chat/completions")
}

func (p *AgentPool) processEnrichFrontmatterWithURL(job Job, apiURL string) {
	apiKey, model := p.resolveCredentials(job)
	if apiKey == "" {
		p.logger.Warn("API key not configured. Agent cannot enrich frontmatter.", zap.Int64("article_id", job.ArticleID))
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
	bodyRunes := []rune(body)
	if len(bodyRunes) > 8000 {
		body = string(bodyRunes[:8000]) // rune safe truncation
	}

	// 3. Ask LLM to generate OKF frontmatter
	prompt := fmt.Sprintf(`You are an expert knowledge curator. Analyze the following article and extract metadata following the Open Knowledge Format (OKF) specification.
Requirements:
- "type": A descriptive content type like "Reference Article", "News", "Documentation", "Playbook", "Tutorial", or "Essay".
- "title": Clean, readable, informative title.
- "description": A concise single sentence summarizing the core insight.
- "resource": Original source URL if known or found in text, otherwise empty string.
- "tags": 3 to 5 lowercase keyword tags.

Article Content:
%s`, body)

	apiMsgs := []interface{}{
		map[string]string{
			"role":    "user",
			"content": prompt,
		},
	}

	reqPayload := enricherOpenRouterRequest{
		Model:    model,
		Messages: apiMsgs,
		ResponseFormat: &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchemaDefinition{
				Name:   "okf_frontmatter",
				Strict: true,
				Schema: jsonSchemaField{
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
				},
			},
		},
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

	// Try up to 2 times with backoff on in-flight budget limits
	for attempt := 0; attempt < 2; attempt++ {
		req, _ := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyJSON))
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

	rawJSON := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	if strings.HasPrefix(rawJSON, "```json") {
		rawJSON = strings.TrimPrefix(rawJSON, "```json")
		rawJSON = strings.TrimSuffix(rawJSON, "```")
	}

	var metadata OKFMetadata
	if err := json.Unmarshal([]byte(rawJSON), &metadata); err != nil {
		p.logger.Error("Enricher failed to unmarshal JSON into OKF metadata", zap.Error(err), zap.String("raw", rawJSON))
		return
	}

	for i, tag := range metadata.Tags {
		metadata.Tags[i] = strings.ToLower(strings.TrimSpace(tag))
	}

	metadata.Generated = OKFGeneratedInfo{
		By: "agent/readr-enricher",
		At: time.Now().UTC().Format(time.RFC3339),
	}

	yamlBytes, err := yaml.Marshal(&metadata)
	if err != nil {
		p.logger.Error("Enricher failed to serialize YAML", zap.Error(err))
		return
	}

	// 4. Overwrite the file with the new pristine OKF frontmatter
	newContent := "---\n" + string(yamlBytes) + "---\n\n" + strings.TrimSpace(body) + "\n"
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		p.logger.Error("Enricher could not write file", zap.Error(err))
		return
	}

	p.logger.Info("Successfully enriched article to OKF format!", zap.Int64("article_id", job.ArticleID))
}
