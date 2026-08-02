package sync

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/zakirullin/files.md/server/config"
)

const (
	Rename = "ren"
	Delete = "del"
)

var lock sync.RWMutex

func LogRename(time int64, oldPath, newPath string) {
	lock.Lock()
	defer lock.Unlock()

	file, err := os.OpenFile(path.Join(config.ServerCfg.WorkingDir, "fslog"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	oldPath = url.QueryEscape(oldPath)
	newPath = url.QueryEscape(newPath)
	record := fmt.Sprintf("%d %s %s %s\n", time, Rename, oldPath, newPath)

	file.WriteString(record)
	file.Sync()
}

func LogDelete(time int64, filepath string) {
	lock.Lock()
	defer lock.Unlock()

	file, err := os.OpenFile(path.Join(config.ServerCfg.WorkingDir, "fslog"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	filepath = url.QueryEscape(filepath)
	record := fmt.Sprintf("%d %s %s\n", time, Delete, filepath)

	file.WriteString(record)
	file.Sync()
}

// RenamesLog reads the file system renames log and returns a map of:
// newPath -> oldPath
// AfterTimestamp is inclusive.
func RenamesLog(userID string, afterTimestamp int64) map[string]string {
	lock.RLock()
	defer lock.RUnlock()

	// TODO can we tolerate errors? The worst that happens are duplicates on client side
	file, err := os.Open(path.Join(config.ServerCfg.WorkingDir, "fslog"))
	if err != nil {
		return nil
	}
	defer file.Close()

	logEntries := make(map[string]string)
	scanner := bufio.NewScanner(file)
	userPathPrefix := path.Join(config.ServerCfg.StorageDir, fmt.Sprintf("%d", userID)) + "/"
	for scanner.Scan() {
		line := scanner.Text()
		var timestamp int64
		var op, oldPath, newPath string
		n, err := fmt.Sscanf(line, "%d %s %s %s", &timestamp, &op, &oldPath, &newPath)
		if op != Rename {
			continue
		}
		if err != nil || n != 4 || timestamp < afterTimestamp {
			continue
		}
		oldPath, err = url.QueryUnescape(oldPath)
		if err != nil {
			continue
		}
		newPath, err = url.QueryUnescape(newPath)
		if err != nil {
			continue
		}

		// TODO exclude ../ from log to prevent Filename Traversal attack
		// Or do we need it? Log rename only logs bot's renames, which are bounded by user folders.
		// And if an attacker get access to the log, he would be able to read files anyway.

		if !strings.HasPrefix(oldPath, userPathPrefix) || !strings.HasPrefix(newPath, userPathPrefix) {
			continue
		}
		oldPath = strings.TrimPrefix(oldPath, userPathPrefix)
		newPath = strings.TrimPrefix(newPath, userPathPrefix)

		logEntries[newPath] = oldPath
	}

	return logEntries
}

// DeletesLog reads the file system deletes log and returns a map of:
// path -> deletedAt unix timestamp
// AfterTimestamp is inclusive. If a path was deleted multiple times,
// the latest timestamp wins.
func DeletesLog(userID string, afterTimestamp int64) map[string]int64 {
	lock.RLock()
	defer lock.RUnlock()

	file, err := os.Open(path.Join(config.ServerCfg.WorkingDir, "fslog"))
	if err != nil {
		return nil
	}
	defer file.Close()

	logEntries := make(map[string]int64)
	scanner := bufio.NewScanner(file)
	userPathPrefix := path.Join(config.ServerCfg.StorageDir, fmt.Sprintf("%d", userID)) + "/"
	for scanner.Scan() {
		line := scanner.Text()
		var timestamp int64
		var op, filepath string
		n, err := fmt.Sscanf(line, "%d %s %s", &timestamp, &op, &filepath)
		if op != Delete {
			continue
		}
		if err != nil || n != 3 || timestamp < afterTimestamp {
			continue
		}
		filepath, err = url.QueryUnescape(filepath)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(filepath, userPathPrefix) {
			continue
		}
		filepath = strings.TrimPrefix(filepath, userPathPrefix)

		if existing, ok := logEntries[filepath]; !ok || timestamp > existing {
			logEntries[filepath] = timestamp
		}
	}

	return logEntries
}
