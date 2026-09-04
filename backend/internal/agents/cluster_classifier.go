package agents

import (
	"sort"
	"strings"
	"unicode"

	"example.com/backend/internal/repository"
)

type ClusterCandidate struct {
	Tag         string                     `json:"tag"`
	Articles    []repository.ArticleRecord `json:"articles"`
	ExistingMOC *repository.ArticleRecord  `json:"existing_moc,omitempty"`
}

// DetectClustersFromArticles groups regular non-MOC articles by their tags into ClusterCandidates.
func DetectClustersFromArticles(articles []repository.ArticleRecord, minSize int) []ClusterCandidate {
	if minSize <= 0 {
		minSize = 5
	}

	var regularArticles []repository.ArticleRecord
	existingMOCMap := make(map[string]repository.ArticleRecord)

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
			mocTag := ""
			if a.Tags != "" {
				for _, tag := range strings.Split(a.Tags, ",") {
					cleaned := repository.SanitizeObsidianTag(tag)
					if cleaned != "" && cleaned != "moc" {
						mocTag = cleaned
						break
					}
				}
			}
			if mocTag == "" {
				topicKey := strings.TrimPrefix(lowerTitle, "moc - ")
				topicKey = strings.TrimPrefix(topicKey, "moc: ")
				topicKey = strings.TrimPrefix(topicKey, "moc ")
				mocTag = repository.SanitizeObsidianTag(topicKey)
			}
			if mocTag != "" {
				existingMOCMap[mocTag] = a
			}
		} else {
			regularArticles = append(regularArticles, a)
		}
	}

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
	seenCandidates := make(map[string]bool)

	for tag, items := range tagClusters {
		var existingMOC *repository.ArticleRecord
		if moc, found := existingMOCMap[tag]; found {
			mocCopy := moc
			existingMOC = &mocCopy
		}

		if len(items) >= minSize || existingMOC != nil {
			seenCandidates[tag] = true
			candidates = append(candidates, ClusterCandidate{
				Tag:         tag,
				Articles:    items,
				ExistingMOC: existingMOC,
			})
		}
	}

	// Also ensure any existing MOCs with 0 tagged articles are reconciled
	for tag, moc := range existingMOCMap {
		if !seenCandidates[tag] {
			mocCopy := moc
			candidates = append(candidates, ClusterCandidate{
				Tag:         tag,
				Articles:    tagClusters[tag],
				ExistingMOC: &mocCopy,
			})
			seenCandidates[tag] = true
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i].Articles) != len(candidates[j].Articles) {
			return len(candidates[i].Articles) > len(candidates[j].Articles)
		}
		return candidates[i].Tag < candidates[j].Tag
	})

	return candidates
}

// titleContainsTopic checks if an article's title contains the topic/tag name (case-insensitive and trimmed).
func titleContainsTopic(title, topic string) bool {
	cleanTitle := strings.ToLower(strings.TrimSpace(title))
	cleanTopic := strings.ToLower(strings.TrimSpace(topic))
	if cleanTitle == "" || cleanTopic == "" {
		return false
	}
	cleanTopic = strings.ReplaceAll(cleanTopic, "-", " ")
	cleanTopic = strings.ReplaceAll(cleanTopic, "_", " ")
	cleanTitle = strings.ReplaceAll(cleanTitle, "-", " ")
	cleanTitle = strings.ReplaceAll(cleanTitle, "_", " ")

	if strings.Contains(cleanTopic, " ") {
		return strings.Contains(cleanTitle, cleanTopic)
	}

	words := strings.FieldsFunc(cleanTitle, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for _, w := range words {
		if w == cleanTopic {
			return true
		}
	}
	return false
}

// DeterminePrimaryTopicFolders determines the single primary topic folder for each article.
func DeterminePrimaryTopicFolders(clusters []ClusterCandidate) map[int64]string {
	primaryTopics := make(map[int64]string)
	articleBestTitleMatch := make(map[int64]bool)
	articleTagIndex := make(map[int64]int)
	articleClusterSizes := make(map[int64]int)

	for _, cluster := range clusters {
		clusterSize := len(cluster.Articles)
		topicFolder := cluster.Tag
		if cluster.ExistingMOC != nil && cluster.ExistingMOC.Title != "" {
			cleanTitle := strings.TrimPrefix(cluster.ExistingMOC.Title, "MOC - ")
			cleanTitle = strings.TrimPrefix(cleanTitle, "MOC: ")
			cleanTitle = strings.TrimPrefix(cleanTitle, "MOC ")
			cleanTitle = strings.TrimSpace(cleanTitle)
			if cleanTitle != "" {
				topicFolder = cleanTitle
			}
		}

		for _, a := range cluster.Articles {
			hasTitleMatch := titleContainsTopic(a.Title, cluster.Tag) || (topicFolder != "" && titleContainsTopic(a.Title, topicFolder))

			tagIdx := 999
			for idx, t := range strings.Split(a.Tags, ",") {
				sanitized := repository.SanitizeObsidianTag(strings.TrimSpace(t))
				if strings.EqualFold(sanitized, repository.SanitizeObsidianTag(cluster.Tag)) || (topicFolder != "" && strings.EqualFold(sanitized, repository.SanitizeObsidianTag(topicFolder))) {
					tagIdx = idx
					break
				}
			}

			prevTitleMatch, exists := articleBestTitleMatch[a.ID]
			isBetter := false

			if !exists {
				isBetter = true
			} else if hasTitleMatch && !prevTitleMatch {
				isBetter = true
			} else if hasTitleMatch == prevTitleMatch {
				if clusterSize < articleClusterSizes[a.ID] {
					isBetter = true
				} else if clusterSize == articleClusterSizes[a.ID] {
					if tagIdx < articleTagIndex[a.ID] {
						isBetter = true
					}
				}
			}

			if isBetter {
				primaryTopics[a.ID] = topicFolder
				articleBestTitleMatch[a.ID] = hasTitleMatch
				articleTagIndex[a.ID] = tagIdx
				articleClusterSizes[a.ID] = clusterSize
			}
		}
	}

	return primaryTopics
}
