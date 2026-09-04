package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"example.com/backend/internal/vault"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LibrarianStatus struct {
	Enabled        bool                `json:"enabled"`
	CronExpression string              `json:"cron_expression"`
	MinClusterSize int                 `json:"min_cluster_size"`
	NextRun        *time.Time          `json:"next_run,omitempty"`
	LastRunResult  *LibrarianRunResult `json:"last_run_result,omitempty"`
}

type LibrarianRunResult struct {
	Status           string   `json:"status"`  // "success", "partial", "failed", "skipped"
	Trigger          string   `json:"trigger"` // "cron", "manual"
	ScannedArticles  int      `json:"scanned_articles"`
	ClustersDetected int      `json:"clusters_detected"`
	CreatedMOCs      int      `json:"created_mocs"`
	UpdatedMOCs      int      `json:"updated_mocs"`
	PrunedMOCs       int      `json:"pruned_mocs"`
	ExecutionTimeMs  int64    `json:"execution_time_ms"`
	Errors           []string `json:"errors,omitempty"`
}

type LibrarianRunner struct {
	logger        *zap.Logger
	db            *gorm.DB
	repo          repository.Repository
	dataDir       string
	organizer     *vault.VaultOrganizer
	onGraphUpdate func()
	lastResult    *LibrarianRunResult
	mu            sync.Mutex
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
	return LibrarianStatus{
		Enabled:        enabled,
		CronExpression: cronExpr,
		MinClusterSize: minClusterSize,
		NextRun:        nextRun,
		LastRunResult:  r.lastResult,
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

	return DetectClustersFromArticles(articles, minSize), nil
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

	startTime := time.Now()
	r.logger.Info("Starting Librarian MOC run", zap.String("trigger", trigger))

	apiKey, model, minSize, enabled := r.getSettings()

	result := &LibrarianRunResult{
		Status:  "success",
		Trigger: trigger,
		Errors:  make([]string, 0),
	}
	defer func() {
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		r.lastResult = result
	}()

	if !enabled {
		r.logger.Info("Librarian is disabled in settings, skipping run")
		result.Status = "skipped (disabled)"
		return result, nil
	}

	allArticles, err := r.repo.GetAllArticles(ctx)
	if err != nil {
		r.logger.Error("Failed to fetch articles for librarian run", zap.Error(err))
		result.Status = "failed"
		result.Errors = append(result.Errors, fmt.Sprintf("fetch articles: %v", err))
		return result, err
	}
	result.ScannedArticles = len(allArticles)

	clusters, err := r.DetectClusters(ctx, minSize)
	if err != nil {
		r.logger.Error("Failed to detect clusters", zap.Error(err))
		result.Status = "failed"
		result.Errors = append(result.Errors, fmt.Sprintf("detect clusters: %v", err))
		return result, err
	}

	// Filter clusters by primary topic specificity
	primaryTopicMap := DeterminePrimaryTopicFolders(clusters)
	var filteredClusters []ClusterCandidate
	for _, cluster := range clusters {
		var specificArticles []repository.ArticleRecord
		for _, a := range cluster.Articles {
			primaryFolder := primaryTopicMap[a.ID]
			topicName := cluster.Tag
			if cluster.ExistingMOC != nil && cluster.ExistingMOC.Title != "" {
				cleanTitle := strings.TrimPrefix(cluster.ExistingMOC.Title, "MOC - ")
				cleanTitle = strings.TrimPrefix(cleanTitle, "MOC: ")
				cleanTitle = strings.TrimPrefix(cleanTitle, "MOC ")
				cleanTitle = strings.TrimSpace(cleanTitle)
				if cleanTitle != "" {
					topicName = cleanTitle
				}
			}

			if strings.EqualFold(repository.SanitizeObsidianTag(primaryFolder), repository.SanitizeObsidianTag(topicName)) {
				specificArticles = append(specificArticles, a)
			}
		}

		if len(specificArticles) >= minSize || cluster.ExistingMOC != nil {
			cluster.Articles = specificArticles
			filteredClusters = append(filteredClusters, cluster)
		} else {
			r.logger.Info("Cluster candidate dropped below minSize after primary topic specificity filtering",
				zap.String("tag", cluster.Tag),
				zap.Int("initial_size", len(cluster.Articles)),
				zap.Int("specific_size", len(specificArticles)),
				zap.Int("min_size", minSize),
			)
		}
	}
	clusters = filteredClusters
	result.ClustersDetected = len(clusters)

	// Step 1: Reconcile / Synthesize Clusters
	for _, cluster := range clusters {
		if cluster.ExistingMOC != nil {
			unlinked, existingContent, err := r.getUnlinkedArticles(cluster)
			if err != nil {
				r.logger.Error("Failed to check unlinked articles for cluster", zap.String("tag", cluster.Tag), zap.Error(err))
				result.Status = "partial (some clusters failed)"
				result.Errors = append(result.Errors, fmt.Sprintf("unlinked %s: %v", cluster.Tag, err))
				continue
			}

			activeMemberTitles := make(map[string]bool)
			for _, a := range cluster.Articles {
				activeMemberTitles[a.Title] = true
				activeMemberTitles[strings.ToLower(a.Title)] = true
				displayTitle := strings.ReplaceAll(a.Title, "|", "—")
				activeMemberTitles[displayTitle] = true
				activeMemberTitles[strings.ToLower(displayTitle)] = true
				if a.FilePath != "" {
					base := strings.TrimSuffix(filepath.Base(a.FilePath), ".md")
					activeMemberTitles[base] = true
					activeMemberTitles[strings.ToLower(base)] = true
				}
				sanitized := ingest.SanitizeTitleFilename(a.Title, a.ID)
				activeMemberTitles[sanitized] = true
				activeMemberTitles[strings.ToLower(sanitized)] = true
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

			// Synthesize delta
			if apiKey == "" {
				r.logger.Warn("OpenRouter API key not configured, skipping delta synthesis for cluster", zap.String("tag", cluster.Tag))
				result.Status = "partial (some clusters failed)"
				result.Errors = append(result.Errors, fmt.Sprintf("tag %s: api key not configured", cluster.Tag))
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

			if cluster.ExistingMOC != nil && cluster.ExistingMOC.Title != "" {
				cleanTitle := strings.TrimPrefix(cluster.ExistingMOC.Title, "MOC - ")
				cleanTitle = strings.TrimPrefix(cleanTitle, "MOC: ")
				cleanTitle = strings.TrimPrefix(cleanTitle, "MOC ")
				cleanTitle = strings.TrimSpace(cleanTitle)
				if cleanTitle != "" && cleanTitle != cluster.Tag {
					for _, a := range cluster.Articles {
						if primaryTopicMap[a.ID] == cluster.Tag {
							primaryTopicMap[a.ID] = cleanTitle
						}
					}
				}
			}

			result.UpdatedMOCs++
		} else {
			// Fresh MOC Synthesis
			if apiKey == "" {
				r.logger.Warn("OpenRouter API key not configured, skipping cluster synthesis", zap.String("tag", cluster.Tag))
				result.Status = "partial (some clusters failed)"
				result.Errors = append(result.Errors, fmt.Sprintf("tag %s: api key not configured", cluster.Tag))
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
			if topicTitle != "" && topicTitle != cluster.Tag {
				for _, a := range cluster.Articles {
					if primaryTopicMap[a.ID] == cluster.Tag {
						primaryTopicMap[a.ID] = topicTitle
					}
				}
			}

			result.CreatedMOCs++
		}
	}

	// Step 2: File Articles into Topic Folders
	for articleID, topicTitle := range primaryTopicMap {
		if _, err := r.organizer.FileArticle(ctx, articleID, topicTitle); err != nil {
			r.logger.Warn("Failed to file article into topic folder", zap.Int64("id", articleID), zap.String("topic", topicTitle), zap.Error(err))
		}
	}

	// Step 3: Prune empty/stale MOCs
	pruned, err := r.pruneEmptyMOCs(ctx)
	if err != nil {
		r.logger.Warn("Failed to prune empty MOCs", zap.Error(err))
	}
	result.PrunedMOCs = pruned

	// Clean any dangling empty topic folders
	if err := r.organizer.CleanEmptyFolders(); err != nil {
		r.logger.Warn("Failed to clean empty topic folders", zap.Error(err))
	}

	// Refresh master index
	if err := r.organizer.UpdateMasterIndex(ctx); err != nil {
		r.logger.Warn("Failed to update master index", zap.Error(err))
	}

	if (result.CreatedMOCs > 0 || result.UpdatedMOCs > 0 || result.PrunedMOCs > 0) && r.onGraphUpdate != nil {
		r.onGraphUpdate()
	}

	if len(result.Errors) > 0 && result.CreatedMOCs == 0 && result.UpdatedMOCs == 0 {
		result.Status = "failed"
	}

	r.logger.Info("Librarian MOC run completed",
		zap.String("status", result.Status),
		zap.Int("created", result.CreatedMOCs),
		zap.Int("updated", result.UpdatedMOCs),
		zap.Int("pruned", result.PrunedMOCs),
		zap.Int64("duration_ms", result.ExecutionTimeMs),
	)

	return result, nil
}

func (r *LibrarianRunner) pruneEmptyMOCs(ctx context.Context) (int, error) {
	var mocs []repository.GormArticle
	if err := r.db.WithContext(ctx).Model(&repository.GormArticle{}).
		Where("deleted_at IS NULL").
		Where("title LIKE 'MOC - %' OR title LIKE 'MOC %' OR title LIKE 'MOC:%' OR tags LIKE '%moc%'").
		Find(&mocs).Error; err != nil {
		return 0, fmt.Errorf("failed to list MOCs for pruning: %w", err)
	}

	prunedCount := 0
	for _, moc := range mocs {
		lowerTitle := strings.ToLower(strings.TrimSpace(moc.Title))
		isMOC := strings.HasPrefix(lowerTitle, "moc - ") || strings.HasPrefix(lowerTitle, "moc:") || strings.HasPrefix(lowerTitle, "moc ") || lowerTitle == "moc"
		if !isMOC && moc.Tags != "" {
			for _, tag := range strings.Split(moc.Tags, ",") {
				if strings.TrimSpace(strings.ToLower(tag)) == "moc" {
					isMOC = true
					break
				}
			}
		}
		if !isMOC {
			continue
		}

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

		topicFolder := ""
		relArticle := strings.TrimPrefix(moc.Article, "/")
		parts := strings.Split(relArticle, "/")
		if len(parts) >= 2 && parts[0] == "articles" {
			topicFolder = parts[1]
		}
		if topicFolder == "" && tag != "" {
			topicFolder = tag
		}

		var memberCount int64
		if topicFolder != "" {
			escapedFolder := escapeLikePattern(topicFolder)
			folderPattern := "%/articles/" + escapedFolder + "/%"
			if err := r.db.WithContext(ctx).Model(&repository.GormArticle{}).
				Where("deleted_at IS NULL AND (is_archived = false OR is_archived IS NULL) AND id != ?", moc.ID).
				Where("article LIKE ? ESCAPE '\\' AND title NOT LIKE 'MOC - %' AND title NOT LIKE 'MOC %' AND title NOT LIKE 'MOC:%'", folderPattern).
				Count(&memberCount).Error; err != nil {
				r.logger.Error("Failed to count member notes for MOC pruning", zap.Int64("id", moc.ID), zap.Error(err))
				continue
			}
		}

		var linkCount int64
		if err := r.db.WithContext(ctx).Table("article_links").
			Joins("JOIN articles ON articles.id = article_links.target_id").
			Where("article_links.source_id = ? AND articles.deleted_at IS NULL AND (articles.is_archived = false OR articles.is_archived IS NULL)", moc.ID).
			Count(&linkCount).Error; err != nil {
			r.logger.Error("Failed to count article links for MOC pruning", zap.Int64("id", moc.ID), zap.Error(err))
			continue
		}

		if memberCount > 0 || linkCount > 0 {
			continue
		}

		filePath := filepath.Join(r.dataDir, strings.TrimPrefix(moc.Article, "/"))
		if moc.Article == "" {
			filePath = filepath.Join(r.dataDir, "articles", tag, fmt.Sprintf("MOC - %s.md", tag))
		}

		contentBytes, err := os.ReadFile(filePath)
		if err == nil {
			if HasCustomUserNotes(string(contentBytes)) {
				r.logger.Info("Preserving empty MOC due to custom user notes in Notes & Synthesis", zap.String("title", moc.Title))
				continue
			}
		}

		r.logger.Info("Pruning empty MOC with no member notes or custom user synthesis", zap.String("title", moc.Title), zap.Int64("id", moc.ID))
		_ = os.Remove(filePath)
		r.db.WithContext(ctx).Where("source_id = ? OR target_id = ?", moc.ID, moc.ID).Delete(&repository.GormArticleLink{})
		r.db.WithContext(ctx).Delete(&repository.GormArticle{}, moc.ID)
		r.db.WithContext(ctx).Exec("DELETE FROM articles_fts WHERE rowid = ?", moc.ID)

		prunedCount++
	}

	return prunedCount, nil
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
	destinationPath := filepath.Join(r.dataDir, "articles", folder, targetFilename)
	relArticlePath := fmt.Sprintf("/articles/%s/%s", folder, targetFilename)

	var oldPath string
	existingBody := ""
	if cluster.ExistingMOC != nil && cluster.ExistingMOC.FilePath != "" {
		existingPath := filepath.Join(r.dataDir, strings.TrimPrefix(cluster.ExistingMOC.FilePath, "/"))
		if bytes, err := os.ReadFile(existingPath); err == nil {
			existingBody = string(bytes)
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

	newMarkdown := assembleMOCMarkdown(synthesis, mocTitle, cluster.Tag, existingBody, articleInfoMap)

	dir := filepath.Dir(destinationPath)
	_ = os.MkdirAll(dir, 0755)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(destinationPath)))
	if err := os.WriteFile(tmpFile, []byte(newMarkdown), 0644); err != nil {
		return fmt.Errorf("failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmpFile, destinationPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename tmp file: %w", err)
	}

	if oldPath != "" && oldPath != destinationPath {
		_ = os.Remove(oldPath)
	}

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

	r.db.WithContext(ctx).Exec("DELETE FROM articles_fts WHERE rowid = ?", mocRecord.ID)
	r.db.WithContext(ctx).Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", mocRecord.ID, mocRecord.Title, tags)

	return nil
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

	mocTitle := fmt.Sprintf("MOC - %s", folder)
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

	newMarkdown := applyDeltaPlacements(existingContent, deltaResp.Placements, articleInfoMap)

	dir := filepath.Dir(destinationPath)
	_ = os.MkdirAll(dir, 0755)
	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(destinationPath)))
	if err := os.WriteFile(tmpFile, []byte(newMarkdown), 0644); err != nil {
		return fmt.Errorf("failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmpFile, destinationPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename tmp file: %w", err)
	}

	if oldPath != "" && oldPath != destinationPath {
		_ = os.Remove(oldPath)
	}

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

	for _, p := range deltaResp.Placements {
		if p.ArticleID > 0 && p.ArticleID != mocRecord.ID {
			var existingLink repository.GormArticleLink
			err := r.db.WithContext(ctx).Where("source_id = ? AND target_id = ?", mocRecord.ID, p.ArticleID).First(&existingLink).Error
			if err != nil {
				link := repository.GormArticleLink{
					SourceID: mocRecord.ID,
					TargetID: p.ArticleID,
				}
				r.db.WithContext(ctx).Create(&link)
			}
		}
	}

	// Prune stale article_links
	if cluster.ExistingMOC != nil {
		activeArticleIDs := make(map[int64]bool)
		for _, a := range cluster.Articles {
			activeArticleIDs[a.ID] = true
		}
		var links []repository.GormArticleLink
		r.db.WithContext(ctx).Where("source_id = ?", mocRecord.ID).Find(&links)
		for _, link := range links {
			if !activeArticleIDs[link.TargetID] {
				r.db.WithContext(ctx).Delete(&link)
			}
		}
	}

	r.db.WithContext(ctx).Exec("DELETE FROM articles_fts WHERE rowid = ?", mocRecord.ID)
	r.db.WithContext(ctx).Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", mocRecord.ID, mocRecord.Title, tags)

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

	// Prune stale article_links
	if cluster.ExistingMOC != nil {
		activeArticleIDs := make(map[int64]bool)
		for _, a := range cluster.Articles {
			activeArticleIDs[a.ID] = true
		}
		var links []repository.GormArticleLink
		r.db.WithContext(ctx).Where("source_id = ?", cluster.ExistingMOC.ID).Find(&links)
		for _, link := range links {
			if !activeArticleIDs[link.TargetID] {
				r.db.WithContext(ctx).Delete(&link)
			}
		}
	}

	return nil
}

func (r *LibrarianRunner) getUnlinkedArticles(cluster ClusterCandidate) ([]repository.ArticleRecord, string, error) {
	return getUnlinkedArticles(cluster, r.dataDir)
}

func getUnlinkedArticles(cluster ClusterCandidate, dataDir string) ([]repository.ArticleRecord, string, error) {
	if cluster.ExistingMOC == nil {
		return cluster.Articles, "", nil
	}

	mocFilePath := ""
	if cluster.ExistingMOC.FilePath != "" {
		rel := strings.TrimPrefix(cluster.ExistingMOC.FilePath, "/")
		mocFilePath = filepath.Join(dataDir, rel)
	}

	if mocFilePath == "" || !fileExists(mocFilePath) {
		topicCandidate := cluster.Tag
		if cluster.ExistingMOC.Title != "" {
			topicCandidate = strings.TrimPrefix(cluster.ExistingMOC.Title, "MOC - ")
		}
		fallbackPath := filepath.Join(dataDir, "articles", topicCandidate, fmt.Sprintf("MOC - %s.md", topicCandidate))
		if fileExists(fallbackPath) {
			mocFilePath = fallbackPath
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

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
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
