package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"example.com/backend/internal/repository"
)

func (r *LibrarianRunner) synthesizeCluster(ctx context.Context, cluster ClusterCandidate, apiKey, model, apiURL string) (*MOCSynthesisResponse, error) {
	articleMap := make(map[int64]repository.ArticleRecord)
	var articleListText strings.Builder
	for _, a := range cluster.Articles {
		articleMap[a.ID] = a
		articleListText.WriteString(fmt.Sprintf("- ID: %d, Title: %s\n", a.ID, a.Title))
	}

	prompt := fmt.Sprintf(`You are a Knowledge Base Librarian generating a Map of Content (MOC) / Hub Note for the topic: "%s".

Candidate Articles in this Topic Cluster:
%s

Instructions:
1. Provide a concise, professional 2-3 sentence executive summary explaining what this topic encompasses based on the articles.
2. Group the articles logically into 2-4 thematic sections (e.g., "Foundational Concepts", "Implementations & Tools", "Advanced Topics").
3. For each article item, provide a 1-sentence contextual summary explaining how it fits into that section.
4. Output strictly adhering to the JSON schema. Use professional plain text without emojis.`, cluster.Tag, articleListText.String())

	schema := map[string]interface{}{
		"type": "json_schema",
		"json_schema": map[string]interface{}{
			"name":   "moc_synthesis",
			"strict": true,
			"schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"topic_title": map[string]interface{}{
						"type":        "string",
						"description": "Clean capitalized topic name (e.g. Distributed Systems)",
					},
					"executive_summary": map[string]interface{}{
						"type":        "string",
						"description": "2-3 sentence executive synthesis of this topic cluster.",
					},
					"sections": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"title": map[string]interface{}{
									"type": "string",
								},
								"items": map[string]interface{}{
									"type": "array",
									"items": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"article_id": map[string]interface{}{
												"type": "integer",
											},
											"context_note": map[string]interface{}{
												"type": "string",
											},
										},
										"required":             []string{"article_id", "context_note"},
										"additionalProperties": false,
									},
								},
							},
							"required":             []string{"title", "items"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"topic_title", "executive_summary", "sections"},
				"additionalProperties": false,
			},
		},
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"response_format": schema,
		"temperature":     0.2,
	}

	reqBytes, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://readr.app")
	httpReq.Header.Set("X-Title", "Readr Librarian MOC Synthesizer")

	startTime := time.Now()
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		if r.repo != nil {
			_ = r.repo.RecordPipelineMetric(ctx, &repository.PipelineMetric{
				ArticleID:        0,
				ArticleTitle:     fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
				Model:            model,
				Status:           "failed",
				DurationMs:       time.Since(startTime).Milliseconds(),
				RetryCount:       0,
				PromptTokens:     len(prompt) / 4,
				CompletionTokens: 0,
				TotalTokens:      len(prompt) / 4,
				ErrorMessage:     err.Error(),
				CreatedAt:        startTime,
			})
		}
		return nil, fmt.Errorf("openrouter request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		errMsg := fmt.Sprintf("openrouter returned status %d: %s", resp.StatusCode, string(bodyBytes))
		if r.repo != nil {
			_ = r.repo.RecordPipelineMetric(ctx, &repository.PipelineMetric{
				ArticleID:        0,
				ArticleTitle:     fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
				Model:            model,
				Status:           "failed",
				DurationMs:       time.Since(startTime).Milliseconds(),
				RetryCount:       0,
				PromptTokens:     len(prompt) / 4,
				CompletionTokens: 0,
				TotalTokens:      len(prompt) / 4,
				ErrorMessage:     errMsg,
				CreatedAt:        startTime,
			})
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse openrouter response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openrouter returned empty choices list")
	}

	rawContent := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	var synthesis MOCSynthesisResponse
	if err := json.Unmarshal([]byte(rawContent), &synthesis); err != nil {
		return nil, fmt.Errorf("failed to unmarshal structured synthesis JSON: %w (raw: %s)", err, rawContent)
	}

	promptTokens := 0
	completionTokens := 0
	totalTokens := 0
	if chatResp.Usage != nil {
		promptTokens = chatResp.Usage.PromptTokens
		completionTokens = chatResp.Usage.CompletionTokens
		totalTokens = chatResp.Usage.TotalTokens
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
	} else {
		promptTokens = len(prompt) / 4
		completionTokens = len(rawContent) / 4
		totalTokens = promptTokens + completionTokens
	}

	mocID := int64(0)
	if cluster.ExistingMOC != nil {
		mocID = cluster.ExistingMOC.ID
	}

	if r.repo != nil {
		_ = r.repo.RecordPipelineMetric(ctx, &repository.PipelineMetric{
			ArticleID:        mocID,
			ArticleTitle:     fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
			Model:            model,
			Status:           "success",
			DurationMs:       time.Since(startTime).Milliseconds(),
			RetryCount:       0,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
			CreatedAt:        startTime,
		})
	}

	return &synthesis, nil
}

func (r *LibrarianRunner) synthesizeDeltaCluster(ctx context.Context, cluster ClusterCandidate, unlinked []repository.ArticleRecord, existingContent string, apiKey, model, apiURL string) (*MOCDeltaResponse, error) {
	sections := extractMOCSections(existingContent)
	var sectionsList strings.Builder
	for _, sec := range sections {
		sectionsList.WriteString(fmt.Sprintf("- %s\n", sec))
	}

	var newArticlesList strings.Builder
	for _, a := range unlinked {
		newArticlesList.WriteString(fmt.Sprintf("- ID: %d, Title: %s\n", a.ID, a.Title))
	}

	prompt := fmt.Sprintf(`You are a Knowledge Base Librarian categorizing new notes into an existing Map of Content (MOC) for topic: "%s".

Existing MOC Sections:
%s
New Articles to Place:
%s
Instructions:
1. For each new article, select the most appropriate existing section from the list above.
2. Provide a 1-sentence contextual summary (context_note) explaining how it fits into that section.
3. Output strictly adhering to the JSON schema. Use professional plain text without emojis.`, cluster.Tag, sectionsList.String(), newArticlesList.String())

	schema := map[string]interface{}{
		"type": "json_schema",
		"json_schema": map[string]interface{}{
			"name":   "moc_delta_placement",
			"strict": true,
			"schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"placements": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"article_id": map[string]interface{}{
									"type": "integer",
								},
								"target_section": map[string]interface{}{
									"type": "string",
								},
								"context_note": map[string]interface{}{
									"type": "string",
								},
							},
							"required":             []string{"article_id", "target_section", "context_note"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"placements"},
				"additionalProperties": false,
			},
		},
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"response_format": schema,
		"temperature":     0.2,
	}

	reqBytes, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://readr.app")
	httpReq.Header.Set("X-Title", "Readr Librarian MOC Synthesizer")

	startTime := time.Now()
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		if r.repo != nil {
			mocID := int64(0)
			if cluster.ExistingMOC != nil {
				mocID = cluster.ExistingMOC.ID
			}
			_ = r.repo.RecordPipelineMetric(ctx, &repository.PipelineMetric{
				ArticleID:        mocID,
				ArticleTitle:     fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
				Model:            model,
				Status:           "failed",
				DurationMs:       time.Since(startTime).Milliseconds(),
				RetryCount:       0,
				PromptTokens:     len(prompt) / 4,
				CompletionTokens: 0,
				TotalTokens:      len(prompt) / 4,
				ErrorMessage:     err.Error(),
				CreatedAt:        startTime,
			})
		}
		return nil, fmt.Errorf("openrouter request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		errMsg := fmt.Sprintf("openrouter returned status %d: %s", resp.StatusCode, string(bodyBytes))
		if r.repo != nil {
			mocID := int64(0)
			if cluster.ExistingMOC != nil {
				mocID = cluster.ExistingMOC.ID
			}
			_ = r.repo.RecordPipelineMetric(ctx, &repository.PipelineMetric{
				ArticleID:        mocID,
				ArticleTitle:     fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
				Model:            model,
				Status:           "failed",
				DurationMs:       time.Since(startTime).Milliseconds(),
				RetryCount:       0,
				PromptTokens:     len(prompt) / 4,
				CompletionTokens: 0,
				TotalTokens:      len(prompt) / 4,
				ErrorMessage:     errMsg,
				CreatedAt:        startTime,
			})
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse openrouter response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openrouter returned empty choices list")
	}

	rawContent := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	var deltaResp MOCDeltaResponse
	if err := json.Unmarshal([]byte(rawContent), &deltaResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal structured delta JSON: %w (raw: %s)", err, rawContent)
	}

	promptTokens := 0
	completionTokens := 0
	totalTokens := 0
	if chatResp.Usage != nil {
		promptTokens = chatResp.Usage.PromptTokens
		completionTokens = chatResp.Usage.CompletionTokens
		totalTokens = chatResp.Usage.TotalTokens
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
	} else {
		promptTokens = len(prompt) / 4
		completionTokens = len(rawContent) / 4
		totalTokens = promptTokens + completionTokens
	}

	mocID := int64(0)
	if cluster.ExistingMOC != nil {
		mocID = cluster.ExistingMOC.ID
	}

	if r.repo != nil {
		_ = r.repo.RecordPipelineMetric(ctx, &repository.PipelineMetric{
			ArticleID:        mocID,
			ArticleTitle:     fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
			Model:            model,
			Status:           "success",
			DurationMs:       time.Since(startTime).Milliseconds(),
			RetryCount:       0,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
			CreatedAt:        startTime,
		})
	}

	return &deltaResp, nil
}
