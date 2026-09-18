package cube

import "testing"

func TestFilterLegacyPythonCronContent(t *testing.T) {
	raw := "15 2 * * * /usr/local/bin/keep\r\n" +
		"0 1 * * * /usr/bin/python3 /etc/ablestack/python/ccvm_snap/create_ccvm_snap.py\r\n" +
		"0 1 * * * /usr/bin/python3 /usr/share/ablestack/backup_mysql.py\r\n"

	got, changed := filterLegacyPythonCronContent(raw, ccvmPCSSetupCronMarker, legacyCCVMDBDumpCronMarker)
	if !changed {
		t.Fatal("legacy cron entries should be reported as changed")
	}
	if want := "15 2 * * * /usr/local/bin/keep\n"; got != want {
		t.Fatalf("filtered cron = %q, want %q", got, want)
	}
}

func TestFilterLegacyPythonCronContentNoChange(t *testing.T) {
	raw := "15 2 * * * /usr/local/bin/keep\n"
	got, changed := filterLegacyPythonCronContent(raw, ccvmPCSSetupCronMarker)
	if changed {
		t.Fatal("unrelated cron entry must not be changed")
	}
	if got != raw {
		t.Fatalf("cron = %q, want %q", got, raw)
	}
}
