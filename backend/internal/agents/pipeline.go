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

	"example.com/backend/internal/repository"
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
				ID:    a.ID,
				Title: a.Title,
				Tags:  a.Tags,
			}
		}
	}
	articleTitle := ""
	if articleRecord != nil {
		articleTitle = articleRecord.Title
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

	startTime := time.Now()
	p.logger.Info("Pipeline dispatching single unified LLM request",
		zap.Int64("article_id", job.ArticleID),
		zap.String("model", model),
	)

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

	p.logger.Info("Pipeline received LLM response",
		zap.Int64("article_id", job.ArticleID),
		zap.Duration("duration", time.Since(startTime)),
	)

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

	p.logger.Info("Successfully completed unified pipeline execution and saved file!",
		zap.Int64("article_id", job.ArticleID),
		zap.String("file", filePath),
	)
}
