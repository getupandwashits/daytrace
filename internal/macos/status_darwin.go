//go:build darwin

package macos

/*
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>

typedef struct {
	char *title;
	int disabled;
	int separator;
	int actionable;
} dt_status_entry;

void dtStatusItemInstall(void);
void dtStatusItemSetTitle(const char *title);
void dtStatusItemSetMenu(dt_status_entry *entries, int count);
*/
import "C"

import (
	"sync"
	"unsafe"
)

// MenuEntry is one row of the menu bar menu. The AppKit menu is rebuilt from
// the whole slice every time it changes, so callers pass a complete menu.
type MenuEntry struct {
	Title     string
	Disabled  bool
	Separator bool
	OnSelect  func()
}

var statusMenu struct {
	mu      sync.Mutex
	actions map[string]func()
}

// InstallStatusItem creates the text status item. It must be called on the
// main thread once the application is running.
func InstallStatusItem() { C.dtStatusItemInstall() }

// SetStatusTitle sets the menu bar text, for example "24:59".
func SetStatusTitle(title string) {
	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))
	C.dtStatusItemSetTitle(ctitle)
}

// SetStatusMenu replaces the status item's menu. Entries without OnSelect and
// without Disabled are shown as plain text rows.
func SetStatusMenu(entries []MenuEntry) {
	statusMenu.mu.Lock()
	if statusMenu.actions == nil {
		statusMenu.actions = map[string]func(){}
	}
	for _, entry := range entries {
		if entry.OnSelect != nil {
			// Action titles are fixed, unique labels. Keep callbacks for rows
			// that AppKit may still be showing in an open menu.
			statusMenu.actions[entry.Title] = entry.OnSelect
		}
	}
	statusMenu.mu.Unlock()

	var array *C.dt_status_entry
	if len(entries) > 0 {
		array = (*C.dt_status_entry)(C.calloc(C.size_t(len(entries)), C.size_t(unsafe.Sizeof(C.dt_status_entry{}))))
		if array == nil {
			return
		}
		rows := unsafe.Slice(array, len(entries))
		for index, entry := range entries {
			rows[index].title = C.CString(entry.Title)
			if entry.Disabled {
				rows[index].disabled = 1
			}
			if entry.Separator {
				rows[index].separator = 1
			}
			if entry.OnSelect != nil && !entry.Disabled {
				rows[index].actionable = 1
			}
		}
		defer func() {
			for _, row := range unsafe.Slice(array, len(entries)) {
				C.free(unsafe.Pointer(row.title))
			}
			C.free(unsafe.Pointer(array))
		}()
	}

	C.dtStatusItemSetMenu(array, C.int(len(entries)))
}

func runStatusAction(title string) {
	statusMenu.mu.Lock()
	action := statusMenu.actions[title]
	statusMenu.mu.Unlock()
	if action != nil {
		action()
	}
}
