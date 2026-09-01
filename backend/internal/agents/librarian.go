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

	"example.com/backend/internal/repository"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

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
	ExecutionTimeMs  int64    `json:"execution_time_ms"`
	Errors           []string `json:"errors,omitempty"`
}

type LibrarianStatus struct {
	Enabled         bool       `json:"enabled"`
	Cron            string     `json:"cron"`
	MinClusterSize  int        `json:"min_cluster_size"`
	LastRun         *time.Time `json:"last_run,omitempty"`
	NextRun         *time.Time `json:"next_run,omitempty"`
	IsRunning       bool       `json:"is_running"`
	LastResult      *LibrarianRunResult `json:"last_result,omitempty"`
}

type LibrarianRunner struct {
	mu            sync.Mutex
	logger        *zap.Logger
	db            *gorm.DB
	repo          repository.Repository
	dataDir       string
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
			unlinked, _, err := r.getUnlinkedArticles(cluster)
			if err != nil {
				r.logger.Error("Failed to check unlinked articles for cluster", zap.String("tag", cluster.Tag), zap.Error(err))
				result.Errors = append(result.Errors, fmt.Sprintf("unlinked %s: %v", cluster.Tag, err))
				continue
			}
			if len(unlinked) == 0 {
				r.logger.Info(fmt.Sprintf("MOC cluster %s is up-to-date (0 new notes), skipping LLM call", cluster.Tag), zap.String("tag", cluster.Tag))
				continue
			}
		}

		synthesis, err := r.synthesizeCluster(ctx, cluster, apiKey, model, apiURL)
		if err != nil {
			r.logger.Error("Failed to synthesize MOC for cluster", zap.String("tag", cluster.Tag), zap.Error(err))
			result.Errors = append(result.Errors, fmt.Sprintf("tag %s: %v", cluster.Tag, err))
			continue
		}

		if err := r.saveMOC(ctx, cluster, synthesis); err != nil {
			r.logger.Error("Failed to save MOC note", zap.String("tag", cluster.Tag), zap.Error(err))
			result.Errors = append(result.Errors, fmt.Sprintf("save %s: %v", cluster.Tag, err))
			continue
		}

		if cluster.ExistingMOC != nil {
			result.UpdatedMOCs++
		} else {
			result.CreatedMOCs++
		}
	}

	result.ExecutionTimeMs = time.Since(start).Milliseconds()

	if r.onGraphUpdate != nil && (result.CreatedMOCs > 0 || result.UpdatedMOCs > 0) {
		r.onGraphUpdate()
	}

	r.mu.Lock()
	now := time.Now()
	r.lastRun = &now
	r.lastResult = result
	r.mu.Unlock()

	return result, nil
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
				ArticleID:    0,
				ArticleTitle: fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
				Model:        model,
				Status:       "failed",
				DurationMs:   time.Since(startTime).Milliseconds(),
				RetryCount:   0,
				PromptTokens: len(prompt) / 4,
				CompletionTokens: 0,
				TotalTokens:  len(prompt) / 4,
				ErrorMessage: err.Error(),
				CreatedAt:    startTime,
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
				ArticleID:    0,
				ArticleTitle: fmt.Sprintf("[Librarian] MOC - %s", cluster.Tag),
				Model:        model,
				Status:       "failed",
				DurationMs:   time.Since(startTime).Milliseconds(),
				RetryCount:   0,
				PromptTokens: len(prompt) / 4,
				CompletionTokens: 0,
				TotalTokens:  len(prompt) / 4,
				ErrorMessage: errMsg,
				CreatedAt:    startTime,
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

	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil || len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("failed to parse openrouter response: %w", err)
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
		topicTitle = strings.Title(strings.ReplaceAll(cluster.Tag, "-", " "))
	}

	mocTitle := fmt.Sprintf("MOC - %s", topicTitle)
	articlesDir := filepath.Join(r.dataDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	targetFilename := fmt.Sprintf("%s.md", mocTitle)
	filePath := filepath.Join(articlesDir, targetFilename)

	existingBody := ""
	if cluster.ExistingMOC != nil && cluster.ExistingMOC.FilePath != "" {
		existingPath := filepath.Join(r.dataDir, strings.TrimPrefix(cluster.ExistingMOC.FilePath, "/"))
		if bytes, err := os.ReadFile(existingPath); err == nil {
			existingBody = string(bytes)
			filePath = existingPath
			targetFilename = filepath.Base(existingPath)
		}
	}

	allArticles, _ := r.repo.GetAllArticles(ctx)
	articleTitleMap := make(map[int64]string)
	for _, a := range allArticles {
		articleTitleMap[a.ID] = a.Title
	}

	newMarkdown := r.assembleMOCMarkdown(synthesis, mocTitle, cluster.Tag, existingBody, articleTitleMap)

	// Atomic disk write
	dir := filepath.Dir(filePath)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(filePath)))
	if err := os.WriteFile(tmpFile, []byte(newMarkdown), 0644); err != nil {
		return fmt.Errorf("failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename tmp file: %w", err)
	}

	// Update or Create DB Article Record
	relArticlePath := fmt.Sprintf("/articles/%s", targetFilename)
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

func (r *LibrarianRunner) assembleMOCMarkdown(synthesis *MOCSynthesisResponse, mocTitle, tag, existingBody string, articleTitleMap map[int64]string) string {
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
			title, ok := articleTitleMap[item.ArticleID]
			if !ok {
				title = fmt.Sprintf("Article %d", item.ArticleID)
			}
			contextNote := strings.TrimSpace(item.ContextNote)
			if contextNote != "" {
				sb.WriteString(fmt.Sprintf("- [[%s|%s]] — %s\n", title, title, contextNote))
			} else {
				sb.WriteString(fmt.Sprintf("- [[%s|%s]]\n", title, title))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Notes & Synthesis\n")
	sb.WriteString(userNotesContent + "\n")

	return sb.String()
}

var reMOCWikilink = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)

func extractLinkedArticlesFromMOC(mocContent string) map[string]bool {
	linked := make(map[string]bool)
	matches := reMOCWikilink.FindAllStringSubmatch(mocContent, -1)
	for _, m := range matches {
		if len(m) > 1 {
			target := strings.TrimSpace(m[1])
			if target != "" {
				linked[target] = true
				linked[strings.ToLower(target)] = true
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
