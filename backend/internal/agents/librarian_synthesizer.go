package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"example.com/backend/internal/llm"
	"example.com/backend/internal/repository"
)

func (r *LibrarianRunner) synthesizeCluster(ctx context.Context, cluster ClusterCandidate, provider llm.Provider, model string) (*MOCSynthesisResponse, error) {
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

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

	startTime := time.Now()
	temp := 0.2

	mocID := int64(0)
	if cluster.ExistingMOC != nil {
		mocID = cluster.ExistingMOC.ID
	}

	mocSynthesisSchema := &llm.JSONSchemaConfig{
		Name:   "moc_synthesis",
		Strict: true,
		Schema: map[string]interface{}{
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
	}

	resp, err := provider.Complete(ctx, llm.CompletionRequest{
		Model: model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: prompt,
			},
		},
		Temperature:    &temp,
		JSONMode:       true,
		ResponseSchema: mocSynthesisSchema,
	})

	if err != nil {
		if r.repo != nil {
			_ = r.repo.RecordPipelineMetric(context.WithoutCancel(parentCtx), &repository.PipelineMetric{
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
		return nil, fmt.Errorf("provider request failed: %w", err)
	}

	rawContent := strings.TrimSpace(resp.Content)
	cleanJSON := cleanJSONBlock(rawContent)

	var synthesis MOCSynthesisResponse
	if err := json.Unmarshal([]byte(cleanJSON), &synthesis); err != nil {
		return nil, fmt.Errorf("failed to unmarshal structured synthesis JSON: %w (raw: %s)", err, rawContent)
	}

	// Filter sections: discard hallucinated article IDs
	var validSections []MOCSection
	for _, sec := range synthesis.Sections {
		var validItems []MOCItem
		for _, item := range sec.Items {
			if _, exists := articleMap[item.ArticleID]; exists {
				validItems = append(validItems, item)
			}
		}
		sec.Items = validItems
		validSections = append(validSections, sec)
	}
	synthesis.Sections = validSections

	promptTokens := len(prompt) / 4
	completionTokens := len(rawContent) / 4
	totalTokens := promptTokens + completionTokens
	if resp.Usage != nil {
		promptTokens = resp.Usage.PromptTokens
		completionTokens = resp.Usage.CompletionTokens
		totalTokens = resp.Usage.TotalTokens
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
	}

	if r.repo != nil {
		_ = r.repo.RecordPipelineMetric(context.WithoutCancel(parentCtx), &repository.PipelineMetric{
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

func (r *LibrarianRunner) synthesizeDeltaCluster(ctx context.Context, cluster ClusterCandidate, unlinked []repository.ArticleRecord, existingContent string, provider llm.Provider, model string) (*MOCDeltaResponse, error) {
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	unlinkedMap := make(map[int64]repository.ArticleRecord)
	sections := extractMOCSections(existingContent)
	var sectionsList strings.Builder
	for _, sec := range sections {
		sectionsList.WriteString(fmt.Sprintf("- %s\n", sec))
	}

	var newArticlesList strings.Builder
	for _, a := range unlinked {
		unlinkedMap[a.ID] = a
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

	startTime := time.Now()
	temp := 0.2

	mocID := int64(0)
	if cluster.ExistingMOC != nil {
		mocID = cluster.ExistingMOC.ID
	}

	mocDeltaSchema := &llm.JSONSchemaConfig{
		Name:   "moc_delta_placement",
		Strict: true,
		Schema: map[string]interface{}{
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
	}

	resp, err := provider.Complete(ctx, llm.CompletionRequest{
		Model: model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: prompt,
			},
		},
		Temperature:    &temp,
		JSONMode:       true,
		ResponseSchema: mocDeltaSchema,
	})

	if err != nil {
		if r.repo != nil {
			_ = r.repo.RecordPipelineMetric(context.WithoutCancel(parentCtx), &repository.PipelineMetric{
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
		return nil, fmt.Errorf("provider request failed: %w", err)
	}

	rawContent := strings.TrimSpace(resp.Content)
	cleanJSON := cleanJSONBlock(rawContent)

	var deltaResp MOCDeltaResponse
	if err := json.Unmarshal([]byte(cleanJSON), &deltaResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal structured delta JSON: %w (raw: %s)", err, rawContent)
	}

	// Filter placements: discard hallucinated article IDs
	var validPlacements []MOCDeltaPlacement
	for _, p := range deltaResp.Placements {
		if _, exists := unlinkedMap[p.ArticleID]; exists {
			validPlacements = append(validPlacements, p)
		}
	}
	deltaResp.Placements = validPlacements

	promptTokens := len(prompt) / 4
	completionTokens := len(rawContent) / 4
	totalTokens := promptTokens + completionTokens
	if resp.Usage != nil {
		promptTokens = resp.Usage.PromptTokens
		completionTokens = resp.Usage.CompletionTokens
		totalTokens = resp.Usage.TotalTokens
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
	}

	if r.repo != nil {
		_ = r.repo.RecordPipelineMetric(context.WithoutCancel(parentCtx), &repository.PipelineMetric{
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
