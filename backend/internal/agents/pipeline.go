package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/llm"
	"example.com/backend/internal/markdown"
	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (p *AgentPool) processPipeline(job Job) error {
	provider, model := p.resolveProvider(job)
	return p.processPipelineWithProvider(job, provider, model)
}

func (p *AgentPool) processPipelineWithURL(job Job, apiURL string) error {
	apiKey, model := p.resolveCredentials(job)
	baseURL := strings.TrimSuffix(strings.TrimSpace(apiURL), "/chat/completions")
	provider := llm.NewProvider(llm.Config{
		Type:    "custom",
		BaseURL: baseURL,
		APIKey:  apiKey,
	})
	return p.processPipelineWithProvider(job, provider, model)
}

func (p *AgentPool) processPipelineWithProvider(job Job, provider llm.Provider, model string) error {
	if !job.Settings.Summarizer && !job.Settings.Enricher && !job.Settings.Linker {
		p.logger.Info("Pipeline skipped: no stages enabled", zap.Int64("article_id", job.ArticleID))
		return nil
	}

	p.mu.RLock()
	hasInjected := p.provider != nil
	p.mu.RUnlock()

	apiKey, _ := p.resolveCredentials(job)
	if provider == nil || (!hasInjected && strings.ToLower(provider.Name()) != "ollama" && apiKey == "") {
		p.logger.Warn("API key not configured. Agent cannot run pipeline.", zap.Int64("article_id", job.ArticleID))
		return fmt.Errorf("API key not configured")
	}

	var repo repository.Repository
	if p.repo != nil {
		repo = p.repo
	} else if p.db != nil {
		repo = repository.NewGormRepository(p.db)
	}

	// 1. Read article metadata from repo or db
	var articleRecord *repository.ArticleRecord
	if repo != nil {
		if a, err := repo.FindByID(context.Background(), job.ArticleID); err == nil && a != nil {
			articleRecord = a
		}
	}
	if articleRecord == nil && p.db != nil {
		var a repository.GormArticle
		if err := p.db.Table("articles").Where("id = ?", job.ArticleID).First(&a).Error; err == nil {
			articleRecord = &repository.ArticleRecord{
				ID:        a.ID,
				Title:     a.Title,
				FilePath:  a.Article,
				ImagePath: a.Image,
				Tags:      a.Tags,
			}
		}
	}
	articleTitle := ""
	if articleRecord != nil {
		articleTitle = articleRecord.Title
	}

	// 2. Resolve and read markdown file
	filePath := ""
	if articleRecord != nil && articleRecord.FilePath != "" {
		rel := strings.TrimPrefix(articleRecord.FilePath, "/")
		candidate := filepath.Join(p.dataDirectory, rel)
		if _, err := os.Stat(candidate); err == nil {
			filePath = candidate
		}
	}
	if filePath == "" {
		if articleRecord != nil && articleRecord.Title != "" {
			sanitized := ingest.SanitizeTitleFilename(articleRecord.Title, job.ArticleID)
			candidate := filepath.Join(p.dataDirectory, "articles", sanitized)
			if _, err := os.Stat(candidate); err == nil {
				filePath = candidate
			}
		}
	}
	if filePath == "" {
		filePath = filepath.Join(p.dataDirectory, "articles", fmt.Sprintf("%d.md", job.ArticleID))
	}

	if articleTitle == "" && filePath != "" {
		articleTitle = strings.TrimSuffix(filepath.Base(filePath), ".md")
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		p.logger.Error("Pipeline could not read file", zap.Error(err), zap.Int64("article_id", job.ArticleID), zap.String("path", filePath))
		return fmt.Errorf("pipeline could not read file %s: %w", filePath, err)
	}
	content := string(contentBytes)

	// Separate frontmatter and body
	frontmatter := ""
	body := content
	if doc, err := markdown.SplitDocument(content); err == nil && doc.HasFrontmatter {
		if doc.RawYAML != "" {
			frontmatter = "---\n" + doc.RawYAML + "\n---\n"
		}
		body = doc.Body
	}

	sourceURL := extractSourceURLFromFrontmatter(frontmatter)
	if sourceURL == "" && articleRecord != nil {
		sourceURL = articleRecord.SourceURL
	}

	p.logger.Info("Starting unified agent pipeline",
		zap.Int64("article_id", job.ArticleID),
		zap.String("title", articleTitle),
		zap.String("source_url", sourceURL),
		zap.Bool("summarizer", job.Settings.Summarizer),
		zap.Bool("enricher", job.Settings.Enricher),
		zap.Bool("linker", job.Settings.Linker),
	)

	// 3. Retrieve candidates for Linker & Taxonomy for Enricher
	var candidates []repository.ArticleRecord
	if job.Settings.Linker && repo != nil {
		if candList, err := repo.FindCandidates(context.Background(), job.ArticleID, articleTitle, body, 15); err == nil {
			candidates = candList
		}
		p.logger.Info("Pipeline retrieved candidate articles for auto-linking",
			zap.Int64("article_id", job.ArticleID),
			zap.Int("candidate_count", len(candidates)),
		)
	}

	var existingVaultTags []string
	if job.Settings.Enricher && repo != nil {
		if tags, err := repo.FindRelevantTags(context.Background(), job.ArticleID, articleTitle, body, 10); err == nil && len(tags) > 0 {
			existingVaultTags = tags
		}
	}

	// 4. Assemble dynamic schema, prompt, and call provider
	properties, required := buildPipelineSchema(job.Settings)
	prompt := buildPipelinePrompt(job.Settings, body, candidates, existingVaultTags)

	startTime := time.Now()
	p.logger.Info("Pipeline dispatching single unified LLM request",
		zap.Int64("article_id", job.ArticleID),
		zap.String("model", model),
	)

	var metricID int64
	if repo != nil {
		initialMetric := repository.PipelineMetric{
			ArticleID:    job.ArticleID,
			ArticleTitle: articleTitle,
			Model:        model,
			Status:       "running",
			DurationMs:   0,
			RetryCount:   0,
			CreatedAt:    startTime,
		}
		_ = repo.RecordPipelineMetric(context.Background(), &initialMetric)
		metricID = initialMetric.ID
	}

	recordMetric := func(status string, promptTokens, completionTokens, totalTokens int, errMsg string) {
		if repo != nil {
			_ = repo.RecordPipelineMetric(context.Background(), &repository.PipelineMetric{
				ID:               metricID,
				ArticleID:        job.ArticleID,
				ArticleTitle:     articleTitle,
				Model:            model,
				Status:           status,
				DurationMs:       time.Since(startTime).Milliseconds(),
				RetryCount:       0,
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      totalTokens,
				ErrorMessage:     errMsg,
				CreatedAt:        startTime,
			})
		}
	}

	resp, err := provider.Complete(context.Background(), llm.CompletionRequest{
		Model: model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: prompt,
			},
		},
		JSONMode: true,
		ResponseSchema: &llm.JSONSchemaConfig{
			Name:   "unified_pipeline_response",
			Strict: true,
			Schema: jsonSchemaField{
				Type:                 "object",
				Properties:           properties,
				Required:             required,
				AdditionalProperties: false,
			},
		},
	})
	if err != nil {
		p.logger.Error("Pipeline LLM request failed", zap.Error(err), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, err.Error())
		return &ProviderError{Err: fmt.Errorf("pipeline LLM request failed: %w", err)}
	}

	p.logger.Info("Pipeline received LLM response",
		zap.Int64("article_id", job.ArticleID),
		zap.Duration("duration", time.Since(startTime)),
	)

	rawJSON := cleanJSONBlock(resp.Content)
	if rawJSON == "" {
		p.logger.Error("Pipeline LLM response message content was empty", zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, "empty message content from LLM provider")
		return &ProviderError{Err: fmt.Errorf("empty message content from LLM provider")}
	}

	var pipelineResp UnifiedPipelineResponse
	if err := json.Unmarshal([]byte(rawJSON), &pipelineResp); err != nil {
		p.logger.Error("Pipeline failed to unmarshal JSON into UnifiedPipelineResponse", zap.Error(err), zap.String("raw", rawJSON), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, "schema parse error: "+err.Error())
		return fmt.Errorf("pipeline failed to unmarshal JSON into UnifiedPipelineResponse: %w", err)
	}

	promptTokens := len(prompt) / 4
	completionTokens := len(rawJSON) / 4
	totalTokens := promptTokens + completionTokens
	if resp.Usage != nil {
		promptTokens = resp.Usage.PromptTokens
		completionTokens = resp.Usage.CompletionTokens
		totalTokens = resp.Usage.TotalTokens
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
	}

	// 5. In-memory transformations
	// Step 1: Summarizer
	if job.Settings.Summarizer && strings.TrimSpace(pipelineResp.Summary) != "" {
		body = applySummary(body, pipelineResp.Summary)
		p.logger.Info("Pipeline step [1/3]: Generated executive summary",
			zap.Int64("article_id", job.ArticleID),
			zap.String("summary", strings.TrimSpace(pipelineResp.Summary)),
		)
	}

	// Step 2: Linker
	if job.Settings.Linker {
		var injectedLinks []string
		if len(pipelineResp.LinksToInject) > 0 {
			body, injectedLinks = injectLinksIntoBody(body, pipelineResp.LinksToInject, candidates, job.ArticleID, p.db)
		}
		if len(injectedLinks) > 0 {
			p.logger.Info("Pipeline step [2/3]: Injected semantic wikilinks",
				zap.Int64("article_id", job.ArticleID),
				zap.Int("links_count", len(injectedLinks)),
				zap.Strings("links_added", injectedLinks),
			)
		} else {
			p.logger.Info("Pipeline step [2/3]: No new semantic links injected",
				zap.Int64("article_id", job.ArticleID),
			)
		}
	}

	// Step 3: Enricher Frontmatter & Tag Synchronization
	if job.Settings.Enricher && pipelineResp.Frontmatter != nil {
		existingTagsStr := ""
		if articleRecord != nil {
			existingTagsStr = articleRecord.Tags
		}
		mergedTags := mergeArticleTags(existingTagsStr, pipelineResp.Frontmatter.Tags)

		yamlHeader, metadata, err := serializeOKFMetadata(pipelineResp.Frontmatter, mergedTags, sourceURL, articleTitle)
		if err != nil {
			p.logger.Error("Pipeline failed to serialize YAML frontmatter", zap.Error(err))
			return fmt.Errorf("pipeline failed to serialize YAML frontmatter: %w", err)
		}
		frontmatter = yamlHeader

		mergedTagsStr := strings.Join(mergedTags, ", ")
		if p.db != nil {
			txErr := p.db.Transaction(func(tx *gorm.DB) error {
				if repo != nil {
					if err := repo.UpdateArticleTags(context.Background(), job.ArticleID, mergedTagsStr); err != nil {
						return err
					}
				}
				if err := tx.Exec("DELETE FROM articles_fts WHERE rowid = ?", job.ArticleID).Error; err != nil {
					return err
				}
				return tx.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", job.ArticleID, metadata.Title, mergedTagsStr).Error
			})
			if txErr != nil {
				p.logger.Error("Pipeline failed to sync article tags and articles_fts index", zap.Error(txErr), zap.Int64("article_id", job.ArticleID))
			}
		} else if repo != nil {
			if err := repo.UpdateArticleTags(context.Background(), job.ArticleID, mergedTagsStr); err != nil {
				p.logger.Error("Pipeline failed to update article tags", zap.Error(err), zap.Int64("article_id", job.ArticleID))
			}
		}

		p.logger.Info("Pipeline step [3/3]: Enriched OKF frontmatter and synchronized database tags",
			zap.Int64("article_id", job.ArticleID),
			zap.String("type", metadata.Type),
			zap.String("title", metadata.Title),
			zap.Strings("tags", metadata.Tags),
			zap.String("merged_tags", mergedTagsStr),
			zap.String("description", metadata.Description),
		)
	}

	// 6. Build consolidated content
	var newContent string
	if frontmatter != "" {
		newContent = strings.TrimRight(frontmatter, "\n") + "\n\n" + strings.TrimSpace(body) + "\n"
	} else {
		newContent = strings.TrimSpace(body) + "\n"
	}

	// 7. Atomic file write
	dir := filepath.Dir(filePath)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(filePath)))
	if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
		p.logger.Error("Pipeline failed to write tmp markdown file", zap.Error(err))
		recordMetric("failed", promptTokens, completionTokens, 0, "file write error: "+err.Error())
		return fmt.Errorf("pipeline failed to write tmp markdown file: %w", err)
	}
	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			p.logger.Error("Pipeline failed to write markdown file", zap.Error(err))
			recordMetric("failed", promptTokens, completionTokens, 0, "file write error: "+err.Error())
			return fmt.Errorf("pipeline failed to write markdown file: %w", err)
		}
	}

	recordMetric("success", promptTokens, completionTokens, totalTokens, "")

	p.logger.Info("Successfully completed unified pipeline execution and saved file!",
		zap.Int64("article_id", job.ArticleID),
		zap.String("file", filePath),
	)
	return nil
}
