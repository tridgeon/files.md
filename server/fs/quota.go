package fs

import (
	"os"
	//"strconv"
	"strings"
	"sync"

	"github.com/spf13/afero"
)

var (
	storageQuotaMu    sync.RWMutex
	storageQuotaCache = map[string]int64{}
)

func storageUsed(rootPath string, backend afero.Fs) int64 {
	storageQuotaMu.RLock()
	used, ok := storageQuotaCache[rootPath]
	storageQuotaMu.RUnlock()
	if ok {
		return used
	}

	var total int64
	_ = afero.Walk(backend, rootPath, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	storageQuotaMu.Lock()
	storageQuotaCache[rootPath] = total
	storageQuotaMu.Unlock()

	return total
}

func recordQuotaUsage(rootPath string, delta int64) {
	storageQuotaMu.Lock()
	if _, ok := storageQuotaCache[rootPath]; ok {
		storageQuotaCache[rootPath] += delta
	}
	storageQuotaMu.Unlock()
}

func checkQuota(rootPath string, backend afero.Fs, quotaKB int64, contentSize int64) error {
	if quotaKB <= 0 {
		return nil
	}

	if storageUsed(rootPath, backend)+contentSize > quotaKB*1024 {
		return ErrQuotaExceeded
	}

	return nil
}

func isUnlimitedQuota(userID string, unlimitedIDs string) bool {
	if unlimitedIDs == "" {
		return false
	}

	for _, idStr := range strings.Split(unlimitedIDs, ",") {
		id := strings.TrimSpace(idStr)

		if id == userID {
			return true
		}
	}

	return false
}
