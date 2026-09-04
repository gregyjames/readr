package agents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

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
