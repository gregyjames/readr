package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type FileRepository struct {
	mu      sync.RWMutex
	baseDir string
}

func NewFileRepository(baseDir string) *FileRepository {
	_ = os.MkdirAll(baseDir, 0755)
	return &FileRepository{baseDir: baseDir}
}

func sanitizeID(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("empty session id")
	}
	clean := filepath.Base(filepath.Clean(id))
	if clean == "" || clean == "." || clean == ".." || clean == "/" || clean == "\\" {
		return "", fmt.Errorf("invalid session id: %q", id)
	}
	return clean, nil
}

func formatRole(role MessageRole) string {
	switch role {
	case RoleUser:
		return "User"
	case RoleAssistant:
		return "Assistant"
	case RoleSystem:
		return "System"
	default:
		s := string(role)
		if len(s) == 0 {
			return "Unknown"
		}
		return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
	}
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	tmpFile := filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, filename); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	return nil
}

func (r *FileRepository) Save(ctx context.Context, session *ChatSession) error {
	if session == nil {
		return errors.New("session cannot be nil")
	}

	id, err := sanitizeID(session.ID)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure Messages slice is not nil for clean JSON array representation
	if session.Messages == nil {
		session.Messages = make([]Message, 0)
	}

	// Save JSON
	jsonPath := filepath.Join(r.baseDir, id+".json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWriteFile(jsonPath, data, 0644); err != nil {
		return err
	}

	// Save MD
	mdPath := filepath.Join(r.baseDir, id+".md")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("---\ntitle: \"%s\"\ndate: \"%s\"\n---\n\n", session.Title, session.CreatedAt.Format("2006-01-02")))
	for _, m := range session.Messages {
		sb.WriteString(fmt.Sprintf("### %s\n", formatRole(m.Role)))
		sb.WriteString(m.Content + "\n\n")
	}
	return atomicWriteFile(mdPath, []byte(sb.String()), 0644)
}

func (r *FileRepository) Get(ctx context.Context, id string) (*ChatSession, error) {
	id, err := sanitizeID(id)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getLocked(ctx, id)
}

func (r *FileRepository) getLocked(ctx context.Context, id string) (*ChatSession, error) {
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
	r.mu.RLock()
	defer r.mu.RUnlock()

	files, err := os.ReadDir(r.baseDir)
	if err != nil {
		return nil, err
	}
	var sessions []*ChatSession
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) == ".json" {
			id := strings.TrimSuffix(f.Name(), ".json")
			if s, err := r.getLocked(ctx, id); err == nil {
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
	id, err := sanitizeID(id)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	jsonPath := filepath.Join(r.baseDir, id+".json")
	if err := os.Remove(jsonPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}

	mdPath := filepath.Join(r.baseDir, id+".md")
	if err := os.Remove(mdPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
