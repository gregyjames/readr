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
	"strings"
	"time"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/hashicorp/go-retryablehttp"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		p.logger.Error("Pipeline could not read file", zap.Error(err), zap.Int64("article_id", job.ArticleID), zap.String("path", filePath))
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

	p.logger.Info("Starting unified agent pipeline",
		zap.Int64("article_id", job.ArticleID),
		zap.String("title", articleTitle),
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
		if tags, err := repo.GetDistinctTags(context.Background()); err == nil && len(tags) > 0 {
			existingVaultTags = tags
		}
	}

	// 4. Assemble dynamic schema, prompt, and payload
	properties, required := buildPipelineSchema(job.Settings)
	prompt := buildPipelinePrompt(job.Settings, body, candidates, existingVaultTags)
	bodyJSON, err := buildPipelinePayload(model, prompt, properties, required)
	if err != nil {
		p.logger.Error("Pipeline failed to marshal payload", zap.Error(err))
		return
	}

	retryCount := 0
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 10 * time.Second
	retryClient.Logger = nil // suppress stdout default logging; logged via zap
	retryClient.RequestLogHook = func(l retryablehttp.Logger, req *http.Request, retryNumber int) {
		retryCount = retryNumber
	}
	retryClient.HTTPClient.Timeout = 60 * time.Second
	retryClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if len(via) > 0 {
			req.Header.Set("Authorization", via[0].Header.Get("Authorization"))
		}
		return nil
	}
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		}
		if resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusPaymentRequired || resp.StatusCode >= 500) {
			return true, nil
		}
		return false, nil
	}

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
				RetryCount:       retryCount,
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      totalTokens,
				ErrorMessage:     errMsg,
				CreatedAt:        startTime,
			})
		}
	}

	req, err := retryablehttp.NewRequestWithContext(context.Background(), http.MethodPost, apiURL, bytes.NewReader(bodyJSON))
	if err != nil {
		p.logger.Error("Pipeline failed to create HTTP request", zap.Error(err))
		recordMetric("failed", 0, 0, 0, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
	req.Header.Set("X-Title", "Readr Pipeline Agent")

	resp, err := retryClient.Do(req)
	if err != nil {
		p.logger.Error("Pipeline LLM request failed", zap.Error(err), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, err.Error())
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Error("Pipeline failed to read LLM response body", zap.Error(err), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, err.Error())
		return
	}

	if resp.StatusCode != http.StatusOK {
		p.logger.Error("Pipeline LLM request returned non-200 status", zap.Int("status", resp.StatusCode), zap.String("body", string(respBytes)), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBytes)))
		return
	}

	p.logger.Info("Pipeline received LLM response",
		zap.Int64("article_id", job.ArticleID),
		zap.Duration("duration", time.Since(startTime)),
	)

	var llmResp struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string      `json:"message"`
			Code    interface{} `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBytes, &llmResp); err != nil {
		p.logger.Error("Pipeline failed to parse LLM response JSON", zap.Error(err), zap.String("raw_response", string(respBytes)), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, "JSON parse error: "+err.Error())
		return
	}

	if llmResp.Error != nil {
		p.logger.Error("Pipeline received API error from LLM provider", zap.String("error_message", llmResp.Error.Message), zap.String("raw_response", string(respBytes)), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, llmResp.Error.Message)
		return
	}

	if len(llmResp.Choices) == 0 {
		p.logger.Error("Pipeline LLM response contained no choices", zap.String("raw_response", string(respBytes)), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, "no choices returned from LLM provider")
		return
	}

	rawJSON := extractMessageContent(llmResp.Choices[0].Message.Content)
	rawJSON = cleanJSONBlock(rawJSON)

	if rawJSON == "" {
		p.logger.Error("Pipeline LLM response message content was empty", zap.String("finish_reason", llmResp.Choices[0].FinishReason), zap.String("raw_response", string(respBytes)), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, "empty message content from LLM provider")
		return
	}

	var pipelineResp UnifiedPipelineResponse
	if err := json.Unmarshal([]byte(rawJSON), &pipelineResp); err != nil {
		p.logger.Error("Pipeline failed to unmarshal JSON into UnifiedPipelineResponse", zap.Error(err), zap.String("raw", rawJSON), zap.String("raw_response", string(respBytes)), zap.Int64("article_id", job.ArticleID))
		recordMetric("failed", 0, 0, 0, "schema parse error: "+err.Error())
		return
	}

	// Calculate token analytics (exact from API usage if present, or deterministic estimate)
	promptTokens := 0
	completionTokens := 0
	totalTokens := 0
	if llmResp.Usage != nil {
		promptTokens = llmResp.Usage.PromptTokens
		completionTokens = llmResp.Usage.CompletionTokens
		totalTokens = llmResp.Usage.TotalTokens
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
	} else {
		promptTokens = len(prompt) / 4
		completionTokens = len(rawJSON) / 4
		totalTokens = promptTokens + completionTokens
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

		yamlHeader, metadata, err := serializeOKFMetadata(pipelineResp.Frontmatter, mergedTags)
		if err != nil {
			p.logger.Error("Pipeline failed to serialize YAML frontmatter", zap.Error(err))
			return
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
		return
	}
	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			p.logger.Error("Pipeline failed to write markdown file", zap.Error(err))
			recordMetric("failed", promptTokens, completionTokens, 0, "file write error: "+err.Error())
			return
		}
	}

	recordMetric("success", promptTokens, completionTokens, totalTokens, "")

	p.logger.Info("Successfully completed unified pipeline execution and saved file!",
		zap.Int64("article_id", job.ArticleID),
		zap.String("file", filePath),
	)
}
