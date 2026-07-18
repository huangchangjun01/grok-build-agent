package fsnotify

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// FileChange represents a detected file change
type FileChange struct {
	Path      string    `json:"path"`
	Operation string    `json:"operation"` // "create", "write", "remove", "rename", "chmod"
	Timestamp time.Time `json:"timestamp"`
}

// Watcher monitors filesystem changes
type Watcher struct {
	watcher *fsnotify.Watcher
	events  chan FileChange
	done    chan struct{}
	logger  *logrus.Logger

	mu       sync.Mutex
	pending  map[string]*pendingEvent
}

type pendingEvent struct {
	change FileChange
	timer  *time.Timer
}

// NewWatcher creates a new filesystem watcher for the given directory
func NewWatcher(dir string, logger *logrus.Logger) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher: fw,
		events:  make(chan FileChange, 256),
		done:    make(chan struct{}),
		logger:  logger,
		pending: make(map[string]*pendingEvent),
	}

	if err := w.AddDir(dir); err != nil {
		fw.Close()
		return nil, err
	}

	return w, nil
}

// Events returns the channel of file change events
func (w *Watcher) Events() <-chan FileChange {
	return w.events
}

// Start begins watching for filesystem changes
func (w *Watcher) Start(ctx context.Context) {
	go func() {
		defer close(w.events)

		for {
			select {
			case <-w.done:
				return
			case <-ctx.Done():
				return
			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}
				w.handleEvent(event)
			case err, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				w.logger.WithError(err).Warn("fsnotify error")
			}
		}
	}()
}

// Stop stops the watcher
func (w *Watcher) Stop() error {
	close(w.done)
	return w.watcher.Close()
}

// AddDir adds a directory to watch recursively
func (w *Watcher) AddDir(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			w.logger.WithError(err).WithField("path", path).Warn("Error walking path, skipping")
			return nil
		}
		if d.IsDir() {
			if addErr := w.watcher.Add(path); addErr != nil {
				w.logger.WithError(addErr).WithField("dir", path).Warn("Failed to add directory to watcher")
			}
		}
		return nil
	})
}

// operationString maps fsnotify operations to human-readable strings
func operationString(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Write):
		return "write"
	case op.Has(fsnotify.Remove):
		return "remove"
	case op.Has(fsnotify.Rename):
		return "rename"
	case op.Has(fsnotify.Chmod):
		return "chmod"
	default:
		return "unknown"
	}
}

// handleEvent processes a raw fsnotify event with debouncing
func (w *Watcher) handleEvent(event fsnotify.Event) {
	change := FileChange{
		Path:      event.Name,
		Operation: operationString(event.Op),
		Timestamp: time.Now(),
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if pending, exists := w.pending[event.Name]; exists {
		pending.change = change
		pending.timer.Reset(100 * time.Millisecond)
		return
	}

	timer := time.AfterFunc(100*time.Millisecond, func() {
		w.mu.Lock()
		pending, ok := w.pending[event.Name]
		if !ok {
			w.mu.Unlock()
			return
		}
		delete(w.pending, event.Name)
		w.mu.Unlock()

		select {
		case w.events <- pending.change:
		case <-w.done:
		}
	})

	w.pending[event.Name] = &pendingEvent{
		change: change,
		timer:  timer,
	}
}