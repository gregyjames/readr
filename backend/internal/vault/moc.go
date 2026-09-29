package vault

import "strings"

// MocSQLCondition is the SQL fragment used for MOC detection. It trims leading whitespace
var MocSQLCondition = "(TRIM(LOWER(title)) LIKE 'moc - %' OR TRIM(LOWER(title)) LIKE 'moc:%' OR TRIM(LOWER(title)) LIKE 'moc %' OR TRIM(LOWER(title)) = 'moc' OR ',' || REPLACE(LOWER(COALESCE(tags, '')), ' ', '') || ',' LIKE '%,moc,%')"

// IsMOCArticle reports whether an article is a MOC based on its title and tags.
// It trims whitespace from the title, lower‑cases it, and matches the same patterns as the SQL condition.
// Tags are expected to be comma‑separated; a tag equal to "moc" (case‑insensitive) also qualifies.
func IsMOCArticle(title, tags string) bool {
	t := strings.TrimSpace(strings.ToLower(title))
	if strings.HasPrefix(t, "moc - ") || strings.HasPrefix(t, "moc:") || strings.HasPrefix(t, "moc ") || t == "moc" {
		return true
	}
	for _, tag := range strings.Split(tags, ",") {
		if strings.TrimSpace(strings.ToLower(tag)) == "moc" {
			return true
		}
	}
	return false
}
