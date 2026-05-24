package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"strm/internal/db"
	"strm/internal/models"
	"strm/internal/scanner"
)

var ErrTaskRunning = errors.New("task already has an active run")

type Manager struct {
	store   *db.Store
	scanner *scanner.Service
	cron    *cron.Cron

	mu     sync.Mutex
	jobs   map[int64]cron.EntryID
	active map[int64]activeRun
}

type activeRun struct {
	info   models.ActiveRun
	cancel context.CancelFunc
}

func New(store *db.Store, scannerSvc *scanner.Service, loc *time.Location) *Manager {
	if loc == nil {
		loc = time.Local
	}
	return &Manager{
		store:   store,
		scanner: scannerSvc,
		cron:    cron.New(cron.WithLocation(loc)),
		jobs:    map[int64]cron.EntryID{},
		active:  map[int64]activeRun{},
	}
}

func (m *Manager) Start(ctx context.Context) error {
	if err := m.Reload(ctx); err != nil {
		return err
	}
	m.cron.Start()
	return nil
}

func (m *Manager) Stop() {
	ctx := m.cron.Stop()
	<-ctx.Done()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.active {
		run.cancel()
	}
}

func (m *Manager) Reload(ctx context.Context) error {
	tasks, err := m.store.ListTasks(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.jobs {
		m.cron.Remove(id)
	}
	m.jobs = map[int64]cron.EntryID{}
	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		taskID := task.ID
		entryID, err := m.cron.AddFunc(task.Cron, func() {
			_, _ = m.StartRun(context.Background(), taskID, "cron")
		})
		if err != nil {
			return fmt.Errorf("schedule task %d: %w", task.ID, err)
		}
		m.jobs[task.ID] = entryID
	}
	return nil
}

func (m *Manager) StartRun(ctx context.Context, taskID int64, trigger string) (models.Run, error) {
	task, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		return models.Run{}, err
	}
	task, err = scanner.NormalizeTask(task)
	if err != nil {
		return models.Run{}, err
	}
	m.mu.Lock()
	if _, ok := m.active[taskID]; ok {
		m.mu.Unlock()
		return models.Run{}, ErrTaskRunning
	}
	m.mu.Unlock()

	run, err := m.store.CreateRun(ctx, taskID, trigger)
	if err != nil {
		return models.Run{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	active := activeRun{
		info: models.ActiveRun{
			RunID:     run.ID,
			TaskID:    task.ID,
			TaskName:  task.Name,
			Trigger:   trigger,
			StartedAt: run.StartedAt,
		},
		cancel: cancel,
	}
	m.mu.Lock()
	if _, ok := m.active[taskID]; ok {
		m.mu.Unlock()
		cancel()
		_ = m.store.FinishRun(context.Background(), run.ID, models.RunStatusCanceled, models.RunStats{}, "task already running")
		return models.Run{}, ErrTaskRunning
	}
	m.active[taskID] = active
	m.mu.Unlock()

	go m.execute(runCtx, run.ID, task)
	return run, nil
}

func (m *Manager) StopRun(taskID int64) bool {
	m.mu.Lock()
	run, ok := m.active[taskID]
	m.mu.Unlock()
	if !ok {
		return false
	}
	run.cancel()
	return true
}

func (m *Manager) ActiveRuns() []models.ActiveRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.ActiveRun, 0, len(m.active))
	for _, run := range m.active {
		out = append(out, run.info)
	}
	return out
}

func (m *Manager) execute(ctx context.Context, runID int64, task models.Task) {
	stats, err := m.scanner.Run(ctx, runID, task)
	status := models.RunStatusSuccess
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		status = models.RunStatusFailed
		if errors.Is(err, context.Canceled) {
			status = models.RunStatusCanceled
		}
	}
	_ = m.store.FinishRun(context.Background(), runID, status, stats, errMsg)
	m.mu.Lock()
	delete(m.active, task.ID)
	m.mu.Unlock()
}
