package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
)

type TemplateContext struct {
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	Source      string            `json:"source"`
	URL         string            `json:"url"`
	Domain      string            `json:"domain"`
	Content     string            `json:"content"`
	Tags        []string          `json:"tags"`
	TagsStr     string            `json:"tags_str"`
	CoverImage  string            `json:"cover_image"`
	SavedDate   string            `json:"saved_date"`
	Timestamp   int64             `json:"timestamp"`
	Author      string            `json:"author"`
	Description string            `json:"description"`
	SiteName    string            `json:"site_name"`
	OG          map[string]string `json:"og"`
}

type TemplateInfo struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

type TemplateRenderer interface {
	ResolveTemplate(hostname string, override string) string
	RequiresSummary(templatePath string) bool
	Render(ctx context.Context, templatePath string, data TemplateContext) (string, error)
	ListTemplates() ([]TemplateInfo, error)
}

type GonjaTemplateRenderer struct {
	templatesDir string
}

func init() {
	gonja.DefaultConfig.KeepTrailingNewline = true
}

func NewGonjaTemplateRenderer(templatesDir string) *GonjaTemplateRenderer {
	if templatesDir != "" {
		_ = os.MkdirAll(templatesDir, 0755)
	}
	return &GonjaTemplateRenderer{
		templatesDir: templatesDir,
	}
}

func (r *GonjaTemplateRenderer) ResolveTemplate(hostname string, override string) string {
	if r.templatesDir == "" {
		return ""
	}

	overrideClean := strings.ToLower(strings.TrimSpace(override))
	if overrideClean == "none" || overrideClean == "default" || overrideClean == "builtin" {
		return ""
	}
	if overrideClean == "auto" {
		overrideClean = ""
	}

	// 1. Manual override
	if overrideClean != "" {
		cleanedOverride := filepath.Base(overrideClean)
		overrideName := strings.TrimSuffix(cleanedOverride, ".jinja") + ".jinja"
		target := filepath.Join(r.templatesDir, overrideName)
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
			return target
		}
	}

	// 2. Domain matching with hierarchy (e.g. sub.domain.com -> domain.com)
	host := strings.ToLower(strings.TrimSpace(hostname))
	parts := strings.Split(host, ".")

	for i := 0; i < len(parts)-1; i++ {
		candidateDomain := strings.Join(parts[i:], ".")
		target := filepath.Join(r.templatesDir, candidateDomain+".jinja")
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
			return target
		}
	}

	return ""
}

func (r *GonjaTemplateRenderer) RequiresSummary(templatePath string) bool {
	if templatePath == "" {
		return false
	}
	contentBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(contentBytes), "summary")
}

func (r *GonjaTemplateRenderer) Render(ctx context.Context, templatePath string, data TemplateContext) (string, error) {
	tpl, err := gonja.FromFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}

	contextMap := exec.NewContext(map[string]interface{}{
		"title":       data.Title,
		"summary":     data.Summary,
		"source":      data.Source,
		"url":         data.URL,
		"domain":      data.Domain,
		"content":     data.Content,
		"tags":        data.Tags,
		"tags_str":    data.TagsStr,
		"cover_image": data.CoverImage,
		"saved_date":  data.SavedDate,
		"timestamp":   data.Timestamp,
		"author":      data.Author,
		"description": data.Description,
		"site_name":   data.SiteName,
		"og":          data.OG,
	})

	var out strings.Builder
	if err := tpl.Execute(&out, contextMap); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}

	return out.String(), nil
}

func (r *GonjaTemplateRenderer) ListTemplates() ([]TemplateInfo, error) {
	if r.templatesDir == "" {
		return []TemplateInfo{}, nil
	}

	entries, err := os.ReadDir(r.templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TemplateInfo{}, nil
		}
		return nil, err
	}

	var templates []TemplateInfo
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jinja") {
			name := strings.TrimSuffix(entry.Name(), ".jinja")
			templates = append(templates, TemplateInfo{
				Name:     name,
				Filename: entry.Name(),
			})
		}
	}

	return templates, nil
}
