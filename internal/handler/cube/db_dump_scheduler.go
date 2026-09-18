package cube

import (
	"log"
	"strings"
	"sync"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/logging"
)

const (
	autoCCVMDBDumpHour   = 1
	autoCCVMDBDumpMinute = 0
)

var autoCCVMDBDumpSchedulerOnce sync.Once

// AutoCCVMDBDumpBackup starts the daily CCVM database backup scheduler.
func AutoCCVMDBDumpBackup() {
	autoCCVMDBDumpSchedulerOnce.Do(func() {
		go runAutoCCVMDBDumpScheduler()
	})
}

func runAutoCCVMDBDumpScheduler() {
	for {
		next := nextAutoCCVMDBDumpTime(time.Now())
		time.Sleep(time.Until(next))
		triggerAutoCCVMDBDumpBackup(next)
	}
}

func nextAutoCCVMDBDumpTime(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), autoCCVMDBDumpHour, autoCCVMDBDumpMinute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func triggerAutoCCVMDBDumpBackup(scheduleTime time.Time) {
	target, err := resolveDBDumpCCVMTarget()
	if err != nil || !isDBDumpLocalTarget(target) {
		return
	}

	root, err := loadClusterJSONRoot()
	if err != nil {
		logAutoCCVMDBDumpFailure("failed to read cluster.json", err, scheduleTime)
		return
	}
	profile, err := extractSystemProfile(root)
	if err != nil {
		logAutoCCVMDBDumpFailure("failed to read systemProfile", err, scheduleTime)
		return
	}
	if !isBootstrapFlagTrue(profile.Bootstrap.Ccvm) {
		log.Printf("ccvm db dump auto skip: bootstrap.ccvm is not true")
		return
	}

	path, err := instantDBDump(dbDumpDefaultPath)
	if err != nil {
		logAutoCCVMDBDumpFailure("database backup failed", err, scheduleTime)
		return
	}
	logging.AppendJobLog("cube.AutoCCVMDBDumpBackup", "backup_success", "success", "database backup completed", map[string]any{
		"path":         path,
		"scheduled_at": scheduleTime.Format(time.RFC3339),
	})
	log.Printf("ccvm db dump auto success: path=%s", path)
}

func logAutoCCVMDBDumpFailure(message string, err error, scheduleTime time.Time) {
	detail := strings.TrimSpace(message + ": " + err.Error())
	logging.AppendJobLog("cube.AutoCCVMDBDumpBackup", "backup_failed", "error", detail, map[string]any{
		"scheduled_at": scheduleTime.Format(time.RFC3339),
	})
	log.Printf("ccvm db dump auto failed: %s", detail)
}
