package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

const tuiRefreshInterval = 10 * time.Second

// tuiWatchMsg is sent when the items directory changes (or on each poll tick
// when filesystem watching is unavailable).
type tuiWatchMsg struct{}

// tuiStartWatcher starts a filesystem watcher on itemsDir and returns a channel
// that receives a signal whenever any file in that directory changes.  Events
// are coalesced so at most one signal is buffered at a time.  Returns nil if
// fsnotify cannot be initialised (e.g. the directory does not exist), in which
// case the caller should fall back to periodic polling.
func tuiStartWatcher(itemsDir string) <-chan struct{} {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	if err := fw.Add(itemsDir); err != nil {
		_ = fw.Close()
		return nil
	}
	ch := make(chan struct{}, 1)
	go func() {
		defer func() { _ = fw.Close() }()
		for {
			select {
			case _, ok := <-fw.Events:
				if !ok {
					return
				}
				// Coalesce: only buffer one pending signal.
				select {
				case ch <- struct{}{}:
				default:
				}
			case _, ok := <-fw.Errors:
				if !ok {
					return
				}
				// ignore watcher errors; next event will retry
			}
		}
	}()
	return ch
}

// tuiWatchCmd returns a tea.Cmd that fires tuiWatchMsg when the items directory
// changes.  If watchCh is non-nil it blocks until the watcher goroutine sends a
// signal; otherwise it falls back to a periodic poll at tuiRefreshInterval.
func tuiWatchCmd(watchCh <-chan struct{}) tea.Cmd {
	if watchCh != nil {
		return func() tea.Msg {
			<-watchCh
			return tuiWatchMsg{}
		}
	}
	return tea.Tick(tuiRefreshInterval, func(time.Time) tea.Msg {
		return tuiWatchMsg{}
	})
}
