package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileRepository struct {
	baseDir string
}

func NewFileRepository(baseDir string) *FileRepository {
	os.MkdirAll(baseDir, 0755)
	return &FileRepository{baseDir: baseDir}
}

func (r *FileRepository) Save(ctx context.Context, session *ChatSession) error {
	// Ensure Messages slice is not nil for clean JSON array representation
	if session.Messages == nil {
		session.Messages = make([]Message, 0)
	}

	// Save JSON
	jsonPath := filepath.Join(r.baseDir, session.ID+".json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		return err
	}

	// Save MD
	mdPath := filepath.Join(r.baseDir, session.ID+".md")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("---\ntitle: \"%s\"\ndate: \"%s\"\n---\n\n", session.Title, session.CreatedAt.Format("2006-01-02")))
	for _, m := range session.Messages {
		if m.Role == RoleUser {
			sb.WriteString("### User\n")
		} else {
			sb.WriteString(fmt.Sprintf("### %s\n", strings.Title(string(m.Role))))
		}
		sb.WriteString(m.Content + "\n\n")
	}
	return os.WriteFile(mdPath, []byte(sb.String()), 0644)
}

func (r *FileRepository) Get(ctx context.Context, id string) (*ChatSession, error) {
	jsonPath := filepath.Join(r.baseDir, id+".json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	if session.Messages == nil {
		session.Messages = make([]Message, 0)
	}
	return &session, nil
}

func (r *FileRepository) List(ctx context.Context) ([]*ChatSession, error) {
	files, err := os.ReadDir(r.baseDir)
	if err != nil {
		return nil, err
	}
	var sessions []*ChatSession
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			id := strings.TrimSuffix(f.Name(), ".json")
			if s, err := r.Get(ctx, id); err == nil {
				sessions = append(sessions, s)
			}
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func (r *FileRepository) Delete(ctx context.Context, id string) error {
	os.Remove(filepath.Join(r.baseDir, id+".json"))
	os.Remove(filepath.Join(r.baseDir, id+".md"))
	return nil
}
