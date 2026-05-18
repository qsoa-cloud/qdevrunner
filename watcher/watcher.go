package watcher

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Restarter is called when a rebuild is needed.
type Restarter interface {
	Restart() error
}

// Watcher watches a directory for Go file changes and triggers rebuilds.
type Watcher struct {
	dir       string
	service   string
	restarter Restarter
	watcher   *fsnotify.Watcher
	mu        sync.Mutex
	timer     *time.Timer
	debounce  time.Duration
}

func New(dir, service string, restarter Restarter) *Watcher {
	return &Watcher{
		dir:       dir,
		service:   service,
		restarter: restarter,
		debounce:  500 * time.Millisecond,
	}
}

// Start begins watching for changes. Blocks until Stop is called.
func (w *Watcher) Start() error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = fsw

	// Walk directory tree and add all directories.
	filepath.Walk(w.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := filepath.Base(path)
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			fsw.Add(path)
		}
		return nil
	})

	log.Printf("[%s] Watching %s for changes", w.service, w.dir)

	for {
		select {
		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			if !isGoFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			w.scheduleRestart()

		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			log.Printf("[%s] Watcher error: %v", w.service, err)
		}
	}
}

func (w *Watcher) Stop() {
	if w.watcher != nil {
		w.watcher.Close()
	}
}

func (w *Watcher) scheduleRestart() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, func() {
		log.Printf("[%s] File change detected, rebuilding...", w.service)
		if err := w.restarter.Restart(); err != nil {
			log.Printf("[%s] Restart failed: %v", w.service, err)
		}
	})
}

func isGoFile(path string) bool {
	return strings.HasSuffix(path, ".go")
}
