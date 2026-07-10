package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFzfConfigKeepsHostileOptionsInertWhenSourced(t *testing.T) {
	zsh := requireZsh(t)
	dir := t.TempDir()
	commandSentinel := filepath.Join(dir, "command-substitution-ran")
	backtickSentinel := filepath.Join(dir, "backtick-ran")
	themeSentinel := filepath.Join(dir, "theme-comment-ran")
	extra := "--cycle --prompt='single quoted' --query=\"double quoted\" " +
		"$(touch " + commandSentinel + ") `touch " + backtickSentinel + "`\n" +
		`--bind='ctrl-a:toggle-all' --literal=$HOME\folder`
	themeName := "dracula\ntouch " + themeSentinel

	output := sourceGeneratedFzfConfig(t, zsh, FzfConfig{DefaultOpts: extra}, themeName)
	for _, sentinel := range []string{commandSentinel, backtickSentinel, themeSentinel} {
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Fatalf("sourcing generated config executed hostile input at %s: %v", sentinel, err)
		}
	}
	if !strings.Contains(output, extra) {
		t.Fatalf("FZF_DEFAULT_OPTS did not preserve literal hostile data:\n%s\nwant substring:\n%s", output, extra)
	}
	if !strings.Contains(output, "--color=dark") {
		t.Fatalf("theme fallback color missing from sourced options: %s", output)
	}
	if !strings.Contains(output, "--border=rounded") {
		t.Fatalf("default border missing from sourced options: %s", output)
	}
}

func TestGenerateFzfConfigPreservesOrdinaryAdditionalFlags(t *testing.T) {
	zsh := requireZsh(t)
	extra := `--cycle --multi --bind='ctrl-a:toggle-all' --query="two words" --tiebreak=end,length`
	output := sourceGeneratedFzfConfig(t, zsh, FzfConfig{
		DefaultOpts:   extra,
		Layout:        "reverse-list",
		BorderStyle:   "sharp",
		Preview:       true,
		PreviewWindow: "up:50%",
		Height:        55,
	}, "dracula")

	for _, want := range []string{
		"--layout=reverse-list",
		"--height=55%",
		"--preview-window=up:50%:wrap",
		"--border=sharp",
		extra,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("sourced FZF_DEFAULT_OPTS = %q, missing %q", output, want)
		}
	}
}

func TestGeneratedOptionsAreAcceptedByInstalledFzf(t *testing.T) {
	zsh := requireZsh(t)
	fzf, err := exec.LookPath("fzf")
	if err != nil {
		t.Skipf("fzf unavailable: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fzf.zsh")
	content := GenerateFzfConfig(FzfConfig{
		DefaultOpts: "--cycle --multi --tiebreak=end,length",
		Height:      40,
		Layout:      "reverse",
		BorderStyle: "rounded",
	}, "dracula")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zsh, "-dfc", `source "$1"; print -r -- $'alpha\nbeta' | "$2" --filter=alpha`, "zsh", path, fzf)
	cmd.Env = []string{"HOME=" + dir, "PATH=" + filepath.Dir(fzf) + ":/usr/bin:/bin"}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("installed fzf rejected generated options: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "alpha") {
		t.Fatalf("installed fzf output = %q, want alpha", output)
	}
}

func TestGenerateFzfConfigFallsBackFromInvalidTypedValues(t *testing.T) {
	zsh := requireZsh(t)
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "typed-value-ran")
	attack := "$(touch " + sentinel + ")"
	content := GenerateFzfConfig(FzfConfig{
		Preview:       true,
		Layout:        "reverse" + attack,
		BorderStyle:   "rounded" + attack,
		PreviewWindow: "right:50%" + attack,
	}, "dracula")
	if strings.Contains(content, attack) {
		t.Fatalf("invalid typed value reached generated config: %s", content)
	}
	output := sourceFzfContent(t, zsh, content)
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("invalid typed value executed: %v", err)
	}
	if strings.Contains(output, "--layout=") {
		t.Fatalf("invalid layout should fall back to default/omitted: %s", output)
	}
	for _, want := range []string{"--border=rounded", "--preview-window=right:50%:wrap"} {
		if !strings.Contains(output, want) {
			t.Fatalf("typed fallback missing %q from %s", want, output)
		}
	}
}

func TestGeneratedFzfConfigZshSyntaxForHostileValues(t *testing.T) {
	zsh := requireZsh(t)
	values := []string{
		`--prompt='single quote'`,
		`--query="double quote"`,
		`$(touch should-not-run)`,
		"`touch should-not-run`",
		"--cycle\n--multi",
		`--query=C:\\path\\with\\backslashes`,
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			content := GenerateFzfConfig(FzfConfig{DefaultOpts: value}, "dracula")
			path := filepath.Join(t.TempDir(), "fzf.zsh")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(zsh, "-n", path).CombinedOutput(); err != nil {
				t.Fatalf("zsh -n failed for %q: %v: %s", value, err, output)
			}
		})
	}
}

func requireZsh(t *testing.T) string {
	t.Helper()
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skipf("zsh unavailable: %v", err)
	}
	return zsh
}

func sourceGeneratedFzfConfig(t *testing.T, zsh string, cfg FzfConfig, themeName string) string {
	t.Helper()
	return sourceFzfContent(t, zsh, GenerateFzfConfig(cfg, themeName))
}

func sourceFzfContent(t *testing.T, zsh, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fzf.zsh")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(zsh, "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("zsh -n generated config: %v: %s", err, output)
	}
	cmd := exec.Command(zsh, "-dfc", `source "$1"; print -r -- "$FZF_DEFAULT_OPTS"`, "zsh", path)
	cmd.Env = []string{"HOME=" + dir, "PATH=/usr/bin:/bin"}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("source generated fzf config: %v: %s", err, output)
	}
	return strings.TrimSuffix(string(output), "\n")
}
