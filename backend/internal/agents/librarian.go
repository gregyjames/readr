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
	"sort"
	"strings"
	"sync"
	"time"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"example.com/backend/internal/vault"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type MOCArticleInfo struct {
	ID       int64
	Title    string
	FilePath string
}

func formatMOCArticleWikilink(info MOCArticleInfo) string {
	fileBase := ""
	if info.FilePath != "" {
		fileBase = strings.TrimSuffix(filepath.Base(info.FilePath), ".md")
	}
	if fileBase == "" || fileBase == fmt.Sprint(info.ID) {
		fileBase = ingest.SanitizeTitleFilename(info.Title, info.ID)
	}

	cleanTitle := strings.TrimSpace(info.Title)
	if cleanTitle == "" {
		cleanTitle = fileBase
	}

	if fileBase == cleanTitle {
		return fmt.Sprintf("[[%s]]", fileBase)
	}

	displayTitle := strings.ReplaceAll(cleanTitle, "|", "—")
	return fmt.Sprintf("[[%s|%s]]", fileBase, displayTitle)
}

type MOCItem struct {
	ArticleID   int64  `json:"article_id"`
	ContextNote string `json:"context_note"`
}

type MOCSection struct {
	Title string    `json:"title"`
	Items []MOCItem `json:"items"`
}

type MOCSynthesisResponse struct {
	TopicTitle       string       `json:"topic_title"`
	ExecutiveSummary string       `json:"executive_summary"`
	Sections         []MOCSection `json:"sections"`
}

type MOCDeltaPlacement struct {
	ArticleID     int64  `json:"article_id"`
	TargetSection string `json:"target_section"`
	ContextNote   string `json:"context_note"`
}

type MOCDeltaResponse struct {
	Placements []MOCDeltaPlacement `json:"placements"`
}

type ClusterCandidate struct {
	Tag         string
	Articles    []repository.ArticleRecord
	ExistingMOC *repository.ArticleRecord
}

type LibrarianRunResult struct {
	Status           string   `json:"status"`
	Trigger          string   `json:"trigger"`
	ScannedArticles  int      `json:"scanned_articles"`
	ClustersDetected int      `json:"clusters_detected"`
	CreatedMOCs      int      `json:"created_mocs"`
	UpdatedMOCs      int      `json:"updated_mocs"`
	PrunedMOCs       int      `json:"pruned_mocs,omitempty"`
	ExecutionTimeMs  int64    `json:"execution_time_ms"`
	Errors           []string `json:"errors,omitempty"`
}

type LibrarianStatus struct {
	Enabled        bool                `json:"enabled"`
	Cron           string              `json:"cron"`
	MinClusterSize int                 `json:"min_cluster_size"`
	LastRun        *time.Time          `json:"last_run,omitempty"`
	NextRun        *time.Time          `json:"next_run,omitempty"`
	IsRunning      bool                `json:"is_running"`
	LastResult     *LibrarianRunResult `json:"last_result,omitempty"`
}

type LibrarianRunner struct {
	mu            sync.Mutex
	logger        *zap.Logger
	db            *gorm.DB
	repo          repository.Repository
	dataDir       string
	organizer     *vault.VaultOrganizer
	onGraphUpdate func()
	lastRun       *time.Time
	lastResult    *LibrarianRunResult
	isRunning     bool
}

func NewLibrarianRunner(logger *zap.Logger, db *gorm.DB, repo repository.Repository, dataDir string, onGraphUpdate func()) *LibrarianRunner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LibrarianRunner{
		logger:        logger,
		db:            db,
		repo:          repo,
		dataDir:       dataDir,
		organizer:     vault.NewVaultOrganizer(dataDir, db, logger),
		onGraphUpdate: onGraphUpdate,
	}
}

func (r *LibrarianRunner) GetStatus(enabled bool, cronExpr string, minClusterSize int, nextRun *time.Time) LibrarianStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	return LibrarianStatus{
		Enabled:        enabled,
		Cron:           cronExpr,
		MinClusterSize: minClusterSize,
		LastRun:        r.lastRun,
		NextRun:        nextRun,
		IsRunning:      r.isRunning,
		LastResult:     r.lastResult,
	}
}

// DetectClusters identifies topics with >= minSize interconnected notes.
func (r *LibrarianRunner) DetectClusters(ctx context.Context, minSize int) ([]ClusterCandidate, error) {
	if minSize <= 0 {
		minSize = 5
	}

	articles, err := r.repo.GetAllArticles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get articles: %w", err)
	}

	// Separate existing MOCs from regular articles
	var regularArticles []repository.ArticleRecord
	existingMOCMap := make(map[string]repository.ArticleRecord) // normalized tag/topic -> MOC article

	for _, a := range articles {
		lowerTitle := strings.ToLower(strings.TrimSpace(a.Title))
		isMOC := strings.HasPrefix(lowerTitle, "moc - ") || strings.HasPrefix(lowerTitle, "moc:") || strings.HasPrefix(lowerTitle, "moc ") || lowerTitle == "moc"
		if !isMOC && a.Tags != "" {
			for _, tag := range strings.Split(a.Tags, ",") {
				if strings.TrimSpace(strings.ToLower(tag)) == "moc" {
					isMOC = true
					break
				}
			}
		}

		if isMOC {
			topicKey := strings.TrimPrefix(lowerTitle, "moc - ")
			topicKey = strings.TrimPrefix(topicKey, "moc: ")
			topicKey = strings.TrimPrefix(topicKey, "moc ")
			topicKey = strings.TrimSpace(topicKey)
			existingMOCMap[topicKey] = a

			if a.Tags != "" {
				for _, tag := range strings.Split(a.Tags, ",") {
					normTag := strings.ToLower(strings.TrimSpace(tag))
					if normTag != "" && normTag != "moc" {
						existingMOCMap[normTag] = a
					}
				}
			}
		} else {
			regularArticles = append(regularArticles, a)
		}
	}

	// Group regular articles by primary tags
	tagClusters := make(map[string][]repository.ArticleRecord)
	for _, a := range regularArticles {
		if a.Tags == "" {
			continue
		}
		seenInArticle := make(map[string]bool)
		for _, tag := range strings.Split(a.Tags, ",") {
			normTag := strings.ToLower(strings.TrimSpace(tag))
			if normTag == "" || normTag == "moc" || seenInArticle[normTag] {
				continue
			}
			seenInArticle[normTag] = true
			tagClusters[normTag] = append(tagClusters[normTag], a)
		}
	}

	var candidates []ClusterCandidate
	for tag, items := range tagClusters {
		if len(items) >= minSize {
			var existingMOC *repository.ArticleRecord
			if moc, found := existingMOCMap[tag]; found {
				mocCopy := moc
				existingMOC = &mocCopy
			}

			candidates = append(candidates, ClusterCandidate{
				Tag:         tag,
				Articles:    items,
				ExistingMOC: existingMOC,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].Articles) > len(candidates[j].Articles)
	})

	return candidates, nil
}

func (r *LibrarianRunner) RunLibrarian(ctx context.Context, trigger string) (*LibrarianRunResult, error) {
	return r.RunLibrarianWithURL(ctx, trigger, "https://openrouter.ai/api/v1/chat/completions")
}

func (r *LibrarianRunner) RunLibrarianWithURL(ctx context.Context, trigger string, apiURL string) (*LibrarianRunResult, error) {
	r.mu.Lock()
	if r.isRunning {
		r.mu.Unlock()
		return nil, fmt.Errorf("librarian is already running")
	}
	r.isRunning = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.isRunning = false
		r.mu.Unlock()
	}()

	start := time.Now()
	result := &LibrarianRunResult{
		Status:  "success",
		Trigger: trigger,
	}

	apiKey, model, minSize, enabled := r.getSettings()
	if !enabled && trigger == "cron" {
		result.Status = "skipped (disabled)"
		return result, nil
	}
	if apiKey == "" {
		result.Status = "skipped (no api key)"
		return result, nil
	}

	clusters, err := r.DetectClusters(ctx, minSize)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}

	result.ClustersDetected = len(clusters)
	allArticles, _ := r.repo.GetAllArticles(ctx)
	result.ScannedArticles = len(allArticles)

	for _, cluster := range clusters {
		if cluster.ExistingMOC != nil {
			unlinked, existingContent, err := r.getUnlinkedArticles(cluster)
			if err != nil {
				r.logger.Error("Failed to check unlinked articles for cluster", zap.String("tag", cluster.Tag), zap.Error(err))
				result.Status = "partial (some clusters failed)"
				result.Errors = append(result.Errors, fmt.Sprintf("unlinked %s: %v", cluster.Tag, err))
				continue
			}

			// Reconcile existing MOC markdown against currently active member notes
			activeMemberTitles := make(map[string]bool)
			for _, a := range cluster.Articles {
				activeMemberTitles[a.Title] = true
				activeMemberTitles[strings.ToLower(a.Title)] = true
				if a.FilePath != "" {
					base := strings.TrimSuffix(filepath.Base(a.FilePath), ".md")
					activeMemberTitles[base] = true
					activeMemberTitles[strings.ToLower(base)] = true
				}
			}
			reconciledContent, linksPruned := ReconcileMOCLinks(existingContent, activeMemberTitles)

			if len(unlinked) == 0 {
				if linksPruned {
					if err := r.saveReconciledMOC(ctx, cluster, reconciledContent); err != nil {
						r.logger.Error("Failed to save reconciled MOC note", zap.String("tag", cluster.Tag), zap.Error(err))
						result.Status = "partial (some clusters failed)"
						result.Errors = append(result.Errors, fmt.Sprintf("save reconciled %s: %v", cluster.Tag, err))
					} else {
						result.UpdatedMOCs++
					}
				} else {
					r.logger.Info(fmt.Sprintf("MOC cluster %s is up-to-date (0 new notes), skipping LLM call", cluster.Tag), zap.String("tag", cluster.Tag))
				}
				continue
			}

			deltaResp, err := r.synthesizeDeltaCluster(ctx, cluster, unlinked, reconciledContent, apiKey, model, apiURL)
			if err != nil {
				r.logger.Error("Failed to synthesize delta MOC for cluster", zap.String("tag", cluster.Tag), zap.Error(err))
				result.Status = "partial (some clusters failed)"
				result.Errors = append(result.Errors, fmt.Sprintf("tag %s: %v", cluster.Tag, err))
				continue
			}

			if err := r.saveDeltaMOC(ctx, cluster, deltaResp, reconciledContent); err != nil {
				r.logger.Error("Failed to save delta MOC note", zap.String("tag", cluster.Tag), zap.Error(err))
				result.Status = "partial (some clusters failed)"
				result.Errors = append(result.Errors, fmt.Sprintf("save delta %s: %v", cluster.Tag, err))
				continue
			}

			topicTitle := cluster.Tag
			if cluster.ExistingMOC.Title != "" {
				cleanTitle := strings.TrimPrefix(cluster.ExistingMOC.Title, "MOC - ")
				cleanTitle = strings.TrimPrefix(cleanTitle, "MOC: ")
				cleanTitle = strings.TrimPrefix(cleanTitle, "MOC ")
				cleanTitle = strings.TrimSpace(cleanTitle)
				if cleanTitle != "" {
					topicTitle = cleanTitle
				}
			}
			var fileErr error
			for _, a := range cluster.Articles {
				if _, err := r.organizer.FileArticle(ctx, a.ID, topicTitle); err != nil {
					r.logger.Error("Failed to file article into topic folder", zap.Int64("article_id", a.ID), zap.String("topic", topicTitle), zap.Error(err))
					result.Status = "partial (some clusters failed)"
					result.Errors = append(result.Errors, fmt.Sprintf("file article %d into %s: %v", a.ID, topicTitle, err))
					fileErr = err
				}
			}
			if fileErr != nil {
				continue
			}

			result.UpdatedMOCs++
			continue
		}

		synthesis, err := r.synthesizeCluster(ctx, cluster, apiKey, model, apiURL)
		if err != nil {
			r.logger.Error("Failed to synthesize MOC for cluster", zap.String("tag", cluster.Tag), zap.Error(err))
			result.Status = "partial (some clusters failed)"
			result.Errors = append(result.Errors, fmt.Sprintf("tag %s: %v", cluster.Tag, err))
			continue
		}

		if err := r.saveMOC(ctx, cluster, synthesis); err != nil {
			r.logger.Error("Failed to save MOC note", zap.String("tag", cluster.Tag), zap.Error(err))
			result.Status = "partial (some clusters failed)"
			result.Errors = append(result.Errors, fmt.Sprintf("save %s: %v", cluster.Tag, err))
			continue
		}

		topicTitle := strings.TrimSpace(synthesis.TopicTitle)
		if topicTitle == "" {
			topicTitle = cluster.Tag
		}
		var fileErr error
		for _, a := range cluster.Articles {
			if _, err := r.organizer.FileArticle(ctx, a.ID, topicTitle); err != nil {
				r.logger.Error("Failed to file article into topic folder", zap.Int64("article_id", a.ID), zap.String("topic", topicTitle), zap.Error(err))
				result.Status = "partial (some clusters failed)"
				result.Errors = append(result.Errors, fmt.Sprintf("file article %d into %s: %v", a.ID, topicTitle, err))
				fileErr = err
			}
		}
		if fileErr != nil {
			continue
		}

		result.CreatedMOCs++
	}

	pruned, err := r.pruneEmptyMOCs(ctx)
	if err != nil {
		r.logger.Error("Failed to prune empty MOCs", zap.Error(err))
		result.Status = "partial (some clusters failed)"
		result.Errors = append(result.Errors, fmt.Sprintf("prune empty MOCs: %v", err))
	} else {
		result.PrunedMOCs = pruned
	}

	if err := r.organizer.CleanEmptyFolders(); err != nil {
		r.logger.Error("Failed to clean empty topic folders", zap.Error(err))
		result.Status = "partial (some clusters failed)"
		result.Errors = append(result.Errors, fmt.Sprintf("clean empty folders: %v", err))
	}
	if err := r.organizer.UpdateMasterIndex(ctx); err != nil {
		r.logger.Error("Failed to update master index", zap.Error(err))
		result.Status = "partial (some clusters failed)"
		result.Errors = append(result.Errors, fmt.Sprintf("update master index: %v", err))
	}

	if len(result.Errors) > 0 {
		if result.CreatedMOCs == 0 && result.UpdatedMOCs == 0 && result.PrunedMOCs == 0 {
			result.Status = "failed"
		} else {
			result.Status = "partial (some clusters failed)"
		}
	}

	result.ExecutionTimeMs = time.Since(start).Milliseconds()

	if r.onGraphUpdate != nil && (result.CreatedMOCs > 0 || result.UpdatedMOCs > 0 || result.PrunedMOCs > 0) {
		r.onGraphUpdate()
	}

	r.mu.Lock()
	now := time.Now()
	r.lastRun = &now
	r.lastResult = result
	r.mu.Unlock()

	return result, nil
}

// pruneEmptyMOCs scans all active MOCs in SQLite. If an MOC has 0 active member notes
// (both via its primary tag cluster and article_links) and contains no custom user-written
// thoughts in ## Notes & Synthesis, it deletes the MOC file from disk and deletes the record from SQLite.
func (r *LibrarianRunner) pruneEmptyMOCs(ctx context.Context) (int, error) {
	if r.db == nil {
		return 0, nil
	}

	var mocs []repository.GormArticle
	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Where("title LIKE 'MOC - %' OR title LIKE 'MOC %' OR title LIKE 'MOC:%' OR tags LIKE '%moc%'").
		Find(&mocs).Error; err != nil {
		return 0, fmt.Errorf("failed to query MOC articles: %w", err)
	}

	prunedCount := 0
	for _, moc := range mocs {
		// 1. Extract tag from MOC
		tag := ""
		tagParts := strings.Split(moc.Tags, ",")
		for _, tp := range tagParts {
			cleaned := repository.SanitizeObsidianTag(tp)
			if cleaned != "" && cleaned != "moc" {
				tag = cleaned
				break
			}
		}
		if tag == "" {
			tag = repository.SanitizeObsidianTag(strings.TrimPrefix(moc.Title, "MOC - "))
		}

		// 2. Count active non-MOC member notes with this tag or in this folder
		var memberCount int64
		if tag != "" {
			r.db.WithContext(ctx).Model(&repository.GormArticle{}).
				Where("deleted_at IS NULL AND id != ?", moc.ID).
				Where("tags LIKE ? OR article LIKE ?", "%"+tag+"%", "%/articles/"+tag+"/%").
				Count(&memberCount)
		}

		// Also check article_links
		var linkCount int64
		r.db.WithContext(ctx).Table("article_links").
			Joins("JOIN articles ON articles.id = article_links.target_id").
			Where("article_links.source_id = ? AND articles.deleted_at IS NULL", moc.ID).
			Count(&linkCount)

		if memberCount > 0 || linkCount > 0 {
			continue // MOC is not empty
		}

		// 3. MOC has 0 member notes. Read file to check for custom user notes in ## Notes & Synthesis
		relPath := strings.TrimPrefix(moc.Article, "/")
		absPath := filepath.Join(r.dataDir, relPath)
		fileBytes, err := os.ReadFile(absPath)
		if err == nil {
			if HasCustomUserNotes(string(fileBytes)) {
				r.logger.Info("Preserving empty MOC containing custom user notes",
					zap.Int64("id", moc.ID),
					zap.String("title", moc.Title),
				)
				continue
			}
		}

		// 4. Safe to prune: remove physical file
		if err == nil {
			_ = os.Remove(absPath)
		}

		// 5. Delete MOC record from SQLite and article_links
		r.db.WithContext(ctx).Where("source_id = ? OR target_id = ?", moc.ID, moc.ID).Delete(&repository.GormArticleLink{})
		r.db.WithContext(ctx).Delete(&moc)
		r.db.WithContext(ctx).Exec("DELETE FROM articles_fts WHERE rowid = ?", moc.ID)

		prunedCount++
		r.logger.Info("Pruned empty automated MOC with 0 notes",
			zap.Int64("id", moc.ID),
			zap.String("title", moc.Title),
			zap.String("file", absPath),
		)
	}

	return prunedCount, nil
}

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

func (r *LibrarianRunner) saveMOC(ctx context.Context, cluster ClusterCandidate, synthesis *MOCSynthesisResponse) error {
	topicTitle := strings.TrimSpace(synthesis.TopicTitle)
	if topicTitle == "" {
		topicTitle = cluster.Tag
	}

	folder, err := r.organizer.EnsureTopicFolder(topicTitle)
	if err != nil {
		return fmt.Errorf("failed to ensure topic folder: %w", err)
	}

	mocTitle := fmt.Sprintf("MOC - %s", folder)
	targetFilename := fmt.Sprintf("MOC - %s.md", folder)
	filePath := filepath.Join(r.dataDir, "articles", folder, targetFilename)
	relArticlePath := fmt.Sprintf("/articles/%s/%s", folder, targetFilename)

	existingBody := ""
	if cluster.ExistingMOC != nil && cluster.ExistingMOC.FilePath != "" {
		existingPath := filepath.Join(r.dataDir, strings.TrimPrefix(cluster.ExistingMOC.FilePath, "/"))
		if bytes, err := os.ReadFile(existingPath); err == nil {
			existingBody = string(bytes)
		}
	}

	allArticles, _ := r.repo.GetAllArticles(ctx)
	articleInfoMap := make(map[int64]MOCArticleInfo)
	for _, a := range allArticles {
		articleInfoMap[a.ID] = MOCArticleInfo{
			ID:       a.ID,
			Title:    a.Title,
			FilePath: a.FilePath,
		}
	}

	newMarkdown := r.assembleMOCMarkdown(synthesis, mocTitle, cluster.Tag, existingBody, articleInfoMap)

	// Atomic disk write
	dir := filepath.Dir(filePath)
	_ = os.MkdirAll(dir, 0755)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(filePath)))
	if err := os.WriteFile(tmpFile, []byte(newMarkdown), 0644); err != nil {
		return fmt.Errorf("failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename tmp file: %w", err)
	}

	// Update or Create DB Article Record
	tags := fmt.Sprintf("moc, %s", cluster.Tag)

	var mocRecord repository.GormArticle
	if cluster.ExistingMOC != nil {
		r.db.WithContext(ctx).First(&mocRecord, cluster.ExistingMOC.ID)
		mocRecord.Title = mocTitle
		mocRecord.Tags = tags
		mocRecord.Article = relArticlePath
		r.db.WithContext(ctx).Save(&mocRecord)
	} else {
		mocRecord = repository.GormArticle{
			Title:   mocTitle,
			Tags:    tags,
			Article: relArticlePath,
		}
		if err := r.db.WithContext(ctx).Create(&mocRecord).Error; err != nil {
			return fmt.Errorf("failed to insert moc article in db: %w", err)
		}
	}

	// Sync article_links
	r.db.WithContext(ctx).Where("source_id = ?", mocRecord.ID).Delete(&repository.GormArticleLink{})
	for _, section := range synthesis.Sections {
		for _, item := range section.Items {
			if item.ArticleID > 0 && item.ArticleID != mocRecord.ID {
				link := repository.GormArticleLink{
					SourceID: mocRecord.ID,
					TargetID: item.ArticleID,
				}
				r.db.WithContext(ctx).Create(&link)
			}
		}
	}

	// Sync FTS
	r.db.WithContext(ctx).Exec("DELETE FROM articles_fts WHERE rowid = ?", mocRecord.ID)
	r.db.WithContext(ctx).Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", mocRecord.ID, mocRecord.Title, tags)

	return nil
}

func (r *LibrarianRunner) assembleMOCMarkdown(synthesis *MOCSynthesisResponse, mocTitle, tag, existingBody string, articleInfoMap map[int64]MOCArticleInfo) string {
	userNotesContent := ""
	if existingBody != "" {
		reNotes := regexp.MustCompile(`(?s)## Notes & Synthesis\s*\n(.*)`)
		if matches := reNotes.FindStringSubmatch(existingBody); len(matches) > 1 {
			userNotesContent = strings.TrimSpace(matches[1])
		}
	}

	if userNotesContent == "" {
		userNotesContent = "<!-- Content below this line is preserved across automated Librarian updates -->\n"
	}

	frontmatterData := map[string]interface{}{
		"type":  "moc",
		"title": mocTitle,
		"tags":  []string{"moc", tag},
		"date":  time.Now().Format("2006-01-02"),
		"generated": map[string]interface{}{
			"by":         "agent/librarian-moc",
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		},
	}

	yamlBytes, _ := yaml.Marshal(frontmatterData)
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(string(yamlBytes))
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# %s\n\n", mocTitle))
	sb.WriteString("## Executive Overview\n")
	sb.WriteString(strings.TrimSpace(synthesis.ExecutiveSummary) + "\n\n")

	sb.WriteString("## Curated Index\n\n")
	for _, section := range synthesis.Sections {
		sb.WriteString(fmt.Sprintf("### %s\n", strings.TrimSpace(section.Title)))
		for _, item := range section.Items {
			info, ok := articleInfoMap[item.ArticleID]
			if !ok {
				info = MOCArticleInfo{ID: item.ArticleID, Title: fmt.Sprintf("Article %d", item.ArticleID)}
			}
			wikilink := formatMOCArticleWikilink(info)
			contextNote := strings.TrimSpace(item.ContextNote)
			if contextNote != "" {
				sb.WriteString(fmt.Sprintf("- %s - %s\n", wikilink, contextNote))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", wikilink))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Notes & Synthesis\n")
	sb.WriteString(userNotesContent + "\n")

	return sb.String()
}

// HasCustomUserNotes returns true if the MOC's ## Notes & Synthesis section contains
// custom user-written text beyond empty whitespace or the default placeholder.
func HasCustomUserNotes(mocContent string) bool {
	reNotes := regexp.MustCompile(`(?s)## Notes & Synthesis\s*\n(.*)`)
	matches := reNotes.FindStringSubmatch(mocContent)
	if len(matches) < 2 {
		return false
	}
	content := strings.TrimSpace(matches[1])
	if content == "" {
		return false
	}

	// Clean out comments like <!-- Content below this line is preserved ... -->
	reComments := regexp.MustCompile(`<!--.*?-->`)
	cleaned := strings.TrimSpace(reComments.ReplaceAllString(content, ""))
	if cleaned == "" {
		return false
	}

	// Check if it's solely the default placeholder text
	placeholder := "*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*"
	placeholderClean := strings.Trim(placeholder, "*_ ")
	trimmedClean := strings.Trim(cleaned, "*_ \n\r\t")

	if trimmedClean == "" || trimmedClean == placeholderClean || strings.EqualFold(trimmedClean, placeholderClean) {
		return false
	}
	return true
}

// ReconcileMOCLinks scans the curated sections of an MOC (excluding ## Notes & Synthesis)
// and prunes any bullet lines containing [[wikilinks]] whose targets are no longer in validMemberTitles.
// Returns the updated markdown and a bool indicating if any lines were pruned.
func ReconcileMOCLinks(mocContent string, validMemberTitles map[string]bool) (string, bool) {
	if mocContent == "" || validMemberTitles == nil {
		return mocContent, false
	}

	parts := strings.SplitN(mocContent, "## Notes & Synthesis", 2)
	curatedPart := parts[0]
	userNotesPart := ""
	if len(parts) == 2 {
		userNotesPart = "## Notes & Synthesis" + parts[1]
	}

	lines := strings.Split(curatedPart, "\n")
	var newLines []string
	changed := false

	// Matches [[Target]] or [[Target|Alias]]
	reWikilink := regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]+)?\]\]`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Only check list item lines that contain a wikilink
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(line, "[[") {
			matches := reWikilink.FindAllStringSubmatch(line, -1)
			if len(matches) > 0 {
				hasValidLink := false
				for _, m := range matches {
					targetTitle := strings.TrimSpace(m[1])
					if validMemberTitles[targetTitle] || validMemberTitles[strings.ToLower(targetTitle)] {
						hasValidLink = true
						break
					}
				}
				if !hasValidLink {
					// Stale link, prune this line
					changed = true
					continue
				}
			}
		}
		newLines = append(newLines, line)
	}

	reconciledCurated := strings.Join(newLines, "\n")
	if userNotesPart != "" {
		return reconciledCurated + userNotesPart, changed
	}
	return reconciledCurated, changed
}

func extractMOCSections(mocContent string) []string {
	var sections []string
	lines := strings.Split(mocContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			sec := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if sec != "" && !strings.EqualFold(sec, "Notes & Synthesis") && !strings.EqualFold(sec, "Executive Overview") && !strings.EqualFold(sec, "Curated Index") {
				sections = append(sections, sec)
			}
		} else if strings.HasPrefix(trimmed, "### ") {
			sec := strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			if sec != "" && !strings.EqualFold(sec, "Notes & Synthesis") && !strings.EqualFold(sec, "Executive Overview") && !strings.EqualFold(sec, "Curated Index") {
				sections = append(sections, sec)
			}
		}
	}
	return sections
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

func applyDeltaPlacements(existingContent string, placements []MOCDeltaPlacement, articleInfoMap map[int64]MOCArticleInfo) string {
	if len(placements) == 0 {
		return existingContent
	}

	type sectionPlacements struct {
		section string
		items   []MOCDeltaPlacement
	}
	var grouped []sectionPlacements
	sectionIdx := make(map[string]int)

	for _, p := range placements {
		sec := strings.TrimSpace(p.TargetSection)
		if sec == "" {
			sec = "Uncategorized"
		}
		normSec := strings.ToLower(sec)
		if idx, exists := sectionIdx[normSec]; exists {
			grouped[idx].items = append(grouped[idx].items, p)
		} else {
			sectionIdx[normSec] = len(grouped)
			grouped = append(grouped, sectionPlacements{
				section: sec,
				items:   []MOCDeltaPlacement{p},
			})
		}
	}

	lines := strings.Split(existingContent, "\n")
	scheduledArticles := make(map[int64]bool)
	scheduledWikilinks := make(map[string]bool)

	for _, g := range grouped {
		headerIdx := -1
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				headerText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
				if strings.EqualFold(headerText, g.section) {
					headerIdx = i
					break
				}
			}
		}

		var itemLines []string
		for _, item := range g.items {
			if scheduledArticles[item.ArticleID] {
				continue
			}

			info, ok := articleInfoMap[item.ArticleID]
			if !ok {
				info = MOCArticleInfo{ID: item.ArticleID, Title: fmt.Sprintf("Article %d", item.ArticleID)}
			}

			wikilink := formatMOCArticleWikilink(info)
			if scheduledWikilinks[wikilink] {
				continue
			}

			fileBase := ""
			if info.FilePath != "" {
				fileBase = strings.TrimSuffix(filepath.Base(info.FilePath), ".md")
			}

			// Prevent duplicate additions if title/file is already referenced anywhere in existing lines
			alreadyPresent := false
			for _, line := range lines {
				if strings.Contains(line, wikilink) ||
					(fileBase != "" && (strings.Contains(line, fmt.Sprintf("[[%s]]", fileBase)) || strings.Contains(line, fmt.Sprintf("[[%s|", fileBase)))) ||
					(info.Title != "" && (strings.Contains(line, fmt.Sprintf("[[%s]]", info.Title)) || strings.Contains(line, fmt.Sprintf("[[%s|", info.Title)))) {
					alreadyPresent = true
					break
				}
			}
			if alreadyPresent {
				continue
			}

			scheduledArticles[item.ArticleID] = true
			scheduledWikilinks[wikilink] = true

			note := strings.TrimSpace(item.ContextNote)
			if note != "" {
				itemLines = append(itemLines, fmt.Sprintf("- %s - %s", wikilink, note))
			} else {
				itemLines = append(itemLines, fmt.Sprintf("- %s", wikilink))
			}
		}

		if len(itemLines) == 0 {
			continue
		}

		if headerIdx != -1 {
			nextHeaderIdx := len(lines)
			for i := headerIdx + 1; i < len(lines); i++ {
				trimmed := strings.TrimSpace(lines[i])
				if strings.HasPrefix(trimmed, "#") {
					nextHeaderIdx = i
					break
				}
			}

			insertIdx := nextHeaderIdx
			for insertIdx > headerIdx+1 && strings.TrimSpace(lines[insertIdx-1]) == "" {
				insertIdx--
			}

			newLines := make([]string, 0, len(lines)+len(itemLines))
			newLines = append(newLines, lines[:insertIdx]...)
			newLines = append(newLines, itemLines...)
			newLines = append(newLines, lines[insertIdx:]...)
			lines = newLines
		} else {
			notesIdx := -1
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") {
					headerText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
					if strings.EqualFold(headerText, "Notes & Synthesis") {
						notesIdx = i
						break
					}
				}
			}

			var newSectionLines []string
			newSectionLines = append(newSectionLines, fmt.Sprintf("## %s", g.section))
			newSectionLines = append(newSectionLines, itemLines...)
			newSectionLines = append(newSectionLines, "")

			if notesIdx != -1 {
				newLines := make([]string, 0, len(lines)+len(newSectionLines))
				newLines = append(newLines, lines[:notesIdx]...)
				newLines = append(newLines, newSectionLines...)
				newLines = append(newLines, lines[notesIdx:]...)
				lines = newLines
			} else {
				lines = append(lines, "")
				lines = append(lines, newSectionLines...)
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (r *LibrarianRunner) saveDeltaMOC(ctx context.Context, cluster ClusterCandidate, deltaResp *MOCDeltaResponse, existingContent string) error {
	topicTitle := cluster.Tag
	if cluster.ExistingMOC != nil && cluster.ExistingMOC.Title != "" {
		cleanTitle := strings.TrimPrefix(cluster.ExistingMOC.Title, "MOC - ")
		cleanTitle = strings.TrimPrefix(cleanTitle, "MOC: ")
		cleanTitle = strings.TrimPrefix(cleanTitle, "MOC ")
		cleanTitle = strings.TrimSpace(cleanTitle)
		if cleanTitle != "" {
			topicTitle = cleanTitle
		}
	}

	folder, err := r.organizer.EnsureTopicFolder(topicTitle)
	if err != nil {
		return fmt.Errorf("failed to ensure topic folder: %w", err)
	}

	targetFilename := fmt.Sprintf("MOC - %s.md", folder)
	destinationPath := filepath.Join(r.dataDir, "articles", folder, targetFilename)
	relArticlePath := fmt.Sprintf("/articles/%s/%s", folder, targetFilename)

	var oldPath string
	if cluster.ExistingMOC != nil && cluster.ExistingMOC.FilePath != "" {
		existingPath := filepath.Join(r.dataDir, strings.TrimPrefix(cluster.ExistingMOC.FilePath, "/"))
		if _, err := os.Stat(existingPath); err == nil {
			oldPath = existingPath
		}
	}

	allArticles, _ := r.repo.GetAllArticles(ctx)
	articleInfoMap := make(map[int64]MOCArticleInfo)
	for _, a := range allArticles {
		articleInfoMap[a.ID] = MOCArticleInfo{
			ID:       a.ID,
			Title:    a.Title,
			FilePath: a.FilePath,
		}
	}

	updatedMarkdown := applyDeltaPlacements(existingContent, deltaResp.Placements, articleInfoMap)

	// Atomic disk write to topic folder destination
	dir := filepath.Dir(destinationPath)
	_ = os.MkdirAll(dir, 0755)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(destinationPath)))
	if err := os.WriteFile(tmpFile, []byte(updatedMarkdown), 0644); err != nil {
		return fmt.Errorf("failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmpFile, destinationPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename tmp file: %w", err)
	}

	// Remove old file location only after new file is durable at destination
	if oldPath != "" && oldPath != destinationPath {
		_ = os.Remove(oldPath)
	}

	// Update DB record with new topic folder path
	var mocRecord repository.GormArticle
	r.db.WithContext(ctx).First(&mocRecord, cluster.ExistingMOC.ID)
	if mocRecord.ID > 0 {
		mocRecord.Article = relArticlePath
		r.db.WithContext(ctx).Save(&mocRecord)
	}

	// Synchronize article_links: delete any targets that are no longer cluster members
	validIDs := make(map[int64]bool)
	for _, a := range cluster.Articles {
		validIDs[a.ID] = true
	}
	for _, p := range deltaResp.Placements {
		if p.ArticleID > 0 {
			validIDs[p.ArticleID] = true
		}
	}
	var existingLinks []repository.GormArticleLink
	r.db.WithContext(ctx).Where("source_id = ?", cluster.ExistingMOC.ID).Find(&existingLinks)
	for _, el := range existingLinks {
		if !validIDs[el.TargetID] {
			r.db.WithContext(ctx).Delete(&el)
		}
	}

	// Add new article_links
	for _, p := range deltaResp.Placements {
		if p.ArticleID > 0 && p.ArticleID != cluster.ExistingMOC.ID {
			var count int64
			r.db.WithContext(ctx).Model(&repository.GormArticleLink{}).
				Where("source_id = ? AND target_id = ?", cluster.ExistingMOC.ID, p.ArticleID).
				Count(&count)
			if count == 0 {
				link := repository.GormArticleLink{
					SourceID: cluster.ExistingMOC.ID,
					TargetID: p.ArticleID,
				}
				r.db.WithContext(ctx).Create(&link)
			}
		}
	}

	return nil
}

func (r *LibrarianRunner) saveReconciledMOC(ctx context.Context, cluster ClusterCandidate, reconciledContent string) error {
	topicTitle := cluster.Tag
	if cluster.ExistingMOC != nil && cluster.ExistingMOC.Title != "" {
		cleanTitle := strings.TrimPrefix(cluster.ExistingMOC.Title, "MOC - ")
		cleanTitle = strings.TrimPrefix(cleanTitle, "MOC: ")
		cleanTitle = strings.TrimPrefix(cleanTitle, "MOC ")
		cleanTitle = strings.TrimSpace(cleanTitle)
		if cleanTitle != "" {
			topicTitle = cleanTitle
		}
	}

	folder, err := r.organizer.EnsureTopicFolder(topicTitle)
	if err != nil {
		return fmt.Errorf("failed to ensure topic folder: %w", err)
	}

	targetFilename := fmt.Sprintf("MOC - %s.md", folder)
	destinationPath := filepath.Join(r.dataDir, "articles", folder, targetFilename)
	relArticlePath := fmt.Sprintf("/articles/%s/%s", folder, targetFilename)

	var oldPath string
	if cluster.ExistingMOC != nil && cluster.ExistingMOC.FilePath != "" {
		existingPath := filepath.Join(r.dataDir, strings.TrimPrefix(cluster.ExistingMOC.FilePath, "/"))
		if _, err := os.Stat(existingPath); err == nil {
			oldPath = existingPath
		}
	}

	dir := filepath.Dir(destinationPath)
	_ = os.MkdirAll(dir, 0755)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(destinationPath)))
	if err := os.WriteFile(tmpFile, []byte(reconciledContent), 0644); err != nil {
		return fmt.Errorf("failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmpFile, destinationPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename tmp file: %w", err)
	}

	if oldPath != "" && oldPath != destinationPath {
		_ = os.Remove(oldPath)
	}

	var mocRecord repository.GormArticle
	r.db.WithContext(ctx).First(&mocRecord, cluster.ExistingMOC.ID)
	if mocRecord.ID > 0 {
		mocRecord.Article = relArticlePath
		r.db.WithContext(ctx).Save(&mocRecord)
	}

	validIDs := make(map[int64]bool)
	for _, a := range cluster.Articles {
		validIDs[a.ID] = true
	}
	var existingLinks []repository.GormArticleLink
	r.db.WithContext(ctx).Where("source_id = ?", cluster.ExistingMOC.ID).Find(&existingLinks)
	for _, el := range existingLinks {
		if !validIDs[el.TargetID] {
			r.db.WithContext(ctx).Delete(&el)
		}
	}

	return nil
}

var reMOCWikilink = regexp.MustCompile(`\[\[([^[\]\n]+?)\]\]`)

func extractLinkedArticlesFromMOC(mocContent string) map[string]bool {
	linked := make(map[string]bool)
	matches := reMOCWikilink.FindAllStringSubmatch(mocContent, -1)
	for _, m := range matches {
		if len(m) > 1 {
			raw := strings.TrimSpace(m[1])
			if raw == "" {
				continue
			}
			// 1. Mark the entire raw string inside [[...]] as linked (preserves literal pipe-containing titles)
			linked[raw] = true
			linked[strings.ToLower(raw)] = true

			// 2. If it is a pipe-delimited [[Target|Alias]], index only the target component
			if strings.Contains(raw, "|") {
				parts := strings.Split(raw, "|")
				target := strings.TrimSpace(parts[0])
				if target != "" {
					linked[target] = true
					linked[strings.ToLower(target)] = true
				}
			}
		}
	}
	return linked
}

func getUnlinkedArticles(cluster ClusterCandidate, dataDir string) ([]repository.ArticleRecord, string, error) {
	if cluster.ExistingMOC == nil {
		return cluster.Articles, "", nil
	}

	mocFilePath := ""
	if cluster.ExistingMOC.FilePath != "" {
		candidate := filepath.Join(dataDir, strings.TrimPrefix(cluster.ExistingMOC.FilePath, "/"))
		if _, err := os.Stat(candidate); err == nil {
			mocFilePath = candidate
		}
	}
	if mocFilePath == "" && cluster.ExistingMOC.Title != "" {
		candidate := filepath.Join(dataDir, "articles", fmt.Sprintf("%s.md", cluster.ExistingMOC.Title))
		if _, err := os.Stat(candidate); err == nil {
			mocFilePath = candidate
		}
	}
	if mocFilePath == "" && cluster.ExistingMOC.ID > 0 {
		candidate := filepath.Join(dataDir, "articles", fmt.Sprintf("%d.md", cluster.ExistingMOC.ID))
		if _, err := os.Stat(candidate); err == nil {
			mocFilePath = candidate
		}
	}

	if mocFilePath == "" {
		return cluster.Articles, "", fmt.Errorf("existing MOC file not found on disk")
	}

	bytes, err := os.ReadFile(mocFilePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read existing MOC file %s: %w", mocFilePath, err)
	}

	existingContent := string(bytes)
	linkedTitles := extractLinkedArticlesFromMOC(existingContent)

	var unlinked []repository.ArticleRecord
	for _, a := range cluster.Articles {
		title := strings.TrimSpace(a.Title)
		normTitle := strings.ToLower(title)
		if linkedTitles[title] || linkedTitles[normTitle] {
			continue
		}
		if a.FilePath != "" {
			base := strings.TrimSuffix(filepath.Base(a.FilePath), ".md")
			if linkedTitles[base] || linkedTitles[strings.ToLower(base)] {
				continue
			}
		}
		unlinked = append(unlinked, a)
	}

	return unlinked, existingContent, nil
}

func (r *LibrarianRunner) getUnlinkedArticles(cluster ClusterCandidate) ([]repository.ArticleRecord, string, error) {
	return getUnlinkedArticles(cluster, r.dataDir)
}

func (r *LibrarianRunner) getSettings() (apiKey, model string, minClusterSize int, enabled bool) {
	settingsPath := filepath.Join(r.dataDir, "settings.json")
	defaults := struct {
		APIKey         string `json:"api_key"`
		Model          string `json:"model"`
		MinClusterSize int    `json:"librarian_min_cluster_size"`
		Enabled        bool   `json:"librarian_enabled"`
	}{
		Model:          "openai/gpt-4o-mini",
		MinClusterSize: 5,
		Enabled:        true,
	}

	data, err := os.ReadFile(settingsPath)
	if err == nil {
		_ = json.Unmarshal(data, &defaults)
	}

	if defaults.MinClusterSize <= 0 {
		defaults.MinClusterSize = 5
	}

	return defaults.APIKey, defaults.Model, defaults.MinClusterSize, defaults.Enabled
}

type LibrarianCronManager struct {
	cron    *cron.Cron
	entryID cron.EntryID
	runner  *LibrarianRunner
	logger  *zap.Logger
	mu      sync.Mutex
}

func NewLibrarianCronManager(runner *LibrarianRunner, logger *zap.Logger) *LibrarianCronManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LibrarianCronManager{
		cron:   cron.New(),
		runner: runner,
		logger: logger,
	}
}

func (m *LibrarianCronManager) Start(cronExpr string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cron != nil {
		m.cron.Stop()
	}
	m.cron = cron.New()
	m.entryID = 0

	if !enabled {
		m.logger.Info("Librarian background cron disabled in settings")
		return nil
	}

	if cronExpr == "" {
		cronExpr = "0 0 * * *"
	}

	id, err := m.cron.AddFunc(cronExpr, func() {
		m.logger.Info("Executing scheduled Librarian MOC background synthesis", zap.String("schedule", cronExpr))
		_, _ = m.runner.RunLibrarian(context.Background(), "cron")
	})
	if err != nil {
		return fmt.Errorf("failed to register cron schedule %q: %w", cronExpr, err)
	}

	m.entryID = id
	m.cron.Start()
	m.logger.Info("Started Librarian background cron scheduler", zap.String("cron", cronExpr))
	return nil
}

func (m *LibrarianCronManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cron.Stop()
}

func (m *LibrarianCronManager) GetNextRun() *time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.cron.Entry(m.entryID)
	if entry.Next.IsZero() {
		return nil
	}
	t := entry.Next
	return &t
}
