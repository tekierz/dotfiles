package pkg

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func aptReceiptFixture(t *testing.T, status, version, batch string) (*AptManager, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("APT_RECEIPT_LOG", log)
	t.Setenv("APT_RECEIPT_STATUS", status)
	t.Setenv("APT_RECEIPT_VERSION", version)
	t.Setenv("APT_RECEIPT_BATCH", batch)
	t.Setenv("APT_RECEIPT_FAIL", "0")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$APT_RECEIPT_LOG"
case "$2" in
 '-f=${Status}') printf '%s' "$APT_RECEIPT_STATUS" ;;
 '-f=${Status}	${Version}') printf '%s\t%s' "$APT_RECEIPT_STATUS" "$APT_RECEIPT_VERSION" ;;
 *) printf '%s' "$APT_RECEIPT_BATCH" ;;
esac
exit "$APT_RECEIPT_FAIL"
`
	for _, name := range []string{"apt", "dpkg-query"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"sudo", "dpkg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return NewAptManager(), log
}

func TestAptReceiptsActualState(t *testing.T) {
	for _, tc := range []struct {
		status    string
		installed bool
	}{
		{"install ok installed", true}, {"hold ok installed", true},
		{"deinstall ok installed", true}, {"purge ok installed", true},
		{"install ok unpacked", false}, {"install ok half-configured", false},
		{"install ok half-installed", false}, {"deinstall ok config-files", false},
		{"install reinstreq installed", false}, {"hold reinstreq installed", false},
		{"install ok triggers-pending", false}, {"install ok triggers-awaited", false},
		{"install ok installed extra", false},
	} {
		t.Run(strings.ReplaceAll(tc.status, " ", "_"), func(t *testing.T) {
			mgr, _ := aptReceiptFixture(t, tc.status, "2:1.0-3", "sample:arm64\t"+tc.status+"\t2:1.0-3\n")
			if got := mgr.IsInstalled("sample:arm64"); got != tc.installed {
				t.Errorf("IsInstalled=%v want %v", got, tc.installed)
			}
			version, err := mgr.GetVersion("sample:arm64")
			if tc.installed {
				if err != nil || version != "2:1.0-3" {
					t.Errorf("GetVersion=%q, %v", version, err)
				}
			} else if err == nil {
				t.Errorf("unhealthy receipt returned version %q", version)
			}
			packages, err := mgr.ListInstalled()
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.installed {
				want = 1
			}
			if len(packages) != want {
				t.Errorf("ListInstalled=%v want %d receipts", packages, want)
			}
		})
	}
}

func TestAptReceiptsBatchMultiarchAndSingleQuery(t *testing.T) {
	mgr, log := aptReceiptFixture(t, "", "", "libc6:amd64\tinstall ok installed\t2.40-1\nlibc6:arm64\thold ok installed\t2.40-2\nbroken\tinstall ok unpacked\t1\n")
	got, err := mgr.ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	want := []Package{{Name: "libc6:amd64", CurrentVersion: "2.40-1", InstalledBy: "apt"}, {Name: "libc6:arm64", CurrentVersion: "2.40-2", InstalledBy: "apt"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("receipts=%v want %v", got, want)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	// The format includes one literal newline; there must be only one -W invocation.
	if strings.Count(string(calls), "-W ") != 1 || !strings.Contains(string(calls), "${binary:Package}") {
		t.Fatalf("unexpected batch query %q", calls)
	}
}

func TestAptReceiptsQueryFailureAndCanceledIndividual(t *testing.T) {
	mgr, log := aptReceiptFixture(t, "hold ok installed", "1", "sample\thold ok installed\t1\n")
	t.Setenv("APT_RECEIPT_FAIL", "1")
	if mgr.IsInstalled("sample") {
		t.Fatal("failed query accepted")
	}
	if version, err := mgr.GetVersion("sample"); err == nil || version != "" {
		t.Fatalf("failed version query=%q %v", version, err)
	}
	if packages, err := mgr.ListInstalled(); err == nil || len(packages) != 0 {
		t.Fatalf("failed batch query=%v %v", packages, err)
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if mgr.IsInstalledContext(ctx, "sample") {
		t.Fatal("canceled query accepted")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("canceled query started: %v", err)
	}
}

func TestAptReceiptsCanceledBatch(t *testing.T) {
	mgr, log := aptReceiptFixture(t, "", "", "sample\thold ok installed\t1\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := mgr.ListInstalledContext(ctx)
	if err != context.Canceled || len(got) != 0 {
		t.Fatalf("canceled batch=%v %v", got, err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("canceled batch started: %v", err)
	}
}

func TestAptReceiptsCancelRunningBatch(t *testing.T) {
	mgr, log := aptReceiptFixture(t, "", "", "")
	query := filepath.Join(filepath.Dir(log), "dpkg-query")
	script := "#!/bin/sh\nprintf started > \"$APT_RECEIPT_LOG\"\nexec /bin/sleep 30\n"
	if err := os.WriteFile(query, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := mgr.ListInstalledContext(ctx); done <- err }()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(log); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("query did not start")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("cancel error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled query did not return")
	}
}

func TestAptReceiptsIgnoreIncompleteRows(t *testing.T) {
	mgr, _ := aptReceiptFixture(t, "", "", "\tinstall ok installed\t1\nsample\tinstall ok installed\t\nmalformed\nextra\tinstall ok installed\t1\textra\n")
	got, err := mgr.ListInstalled()
	if err != nil || len(got) != 0 {
		t.Fatalf("incomplete receipts=%v %v", got, err)
	}
}
