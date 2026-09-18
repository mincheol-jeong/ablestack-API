package cube

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const legacyCCVMDBDumpCronMarker = "backup_mysql.py"

var legacyPythonCronCleanupOnce sync.Once

// AutoLegacyPythonCronCleanup removes jobs replaced by the API's Go schedulers.
func AutoLegacyPythonCronCleanup() {
	legacyPythonCronCleanupOnce.Do(func() {
		if err := cleanupLegacyPythonCrontab(ccvmPCSSetupCronMarker, legacyCCVMDBDumpCronMarker); err != nil {
			log.Printf("legacy python cron cleanup failed: %v", err)
		}
	})
}

func cleanupLegacyPythonCrontab(markers ...string) error {
	raw, err := os.ReadFile(ccvmPCSSetupCronPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	content, changed := filterLegacyPythonCronContent(string(raw), markers...)
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ccvmPCSSetupCronPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(ccvmPCSSetupCronPath), ".ablestack-cron-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, ccvmPCSSetupCronPath); err != nil {
		return err
	}
	if _, err := runPCSCommand(pcsCommandTimeout, "systemctl", "restart", "crond.service"); err != nil {
		return fmt.Errorf("restart crond after legacy cron cleanup: %w", err)
	}
	return nil
}

func filterLegacyPythonCronContent(raw string, markers ...string) (string, bool) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	filtered := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		remove := false
		for _, marker := range markers {
			if marker != "" && strings.Contains(line, marker) {
				remove = true
				changed = true
				break
			}
		}
		if !remove {
			filtered = append(filtered, strings.TrimRight(line, "\r"))
		}
	}
	for len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) == "" {
		filtered = filtered[:len(filtered)-1]
	}
	if len(filtered) == 0 {
		return "", changed
	}
	return strings.Join(filtered, "\n") + "\n", changed
}
