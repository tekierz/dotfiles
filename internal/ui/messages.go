package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
)

// tickMsg is sent on each animation frame
type tickMsg time.Time

// uiTickMsg is a global tick for small UI animations (spinners/widgets).
type uiTickMsg time.Time

// durdrawAvailableMsg indicates if durdraw is available
type durdrawAvailableMsg bool

// animationDoneMsg indicates the animation has finished
type animationDoneMsg struct{}

// installOutputMsg carries output from the installation
type installOutputMsg struct {
	line runner.OutputLine
}

// installDoneMsg indicates installation completed
type installDoneMsg struct {
	err     error
	context string // last few lines of output for error context
}

// installStartMsg triggers installation start
type installStartMsg struct{}

// sudoRequiredMsg indicates sudo is needed
type sudoRequiredMsg struct{}

// sudoCachedMsg indicates sudo credentials are ready
type sudoCachedMsg struct {
	err error
}

// updateCheckDoneMsg indicates the async update check completed
type updateCheckDoneMsg struct {
	updates []pkg.Package
	err     error
}

// installCacheDoneMsg indicates the async install cache loading completed
type installCacheDoneMsg struct {
	installed map[string]bool
}

// updateRunDoneMsg indicates an update operation completed
type updateRunDoneMsg struct {
	results []pkg.UpdateResult
	err     error
}

// installLogMsg carries a single log line from streaming install/update
type installLogMsg struct {
	line string
}

// manageSudoRequiredMsg indicates sudo is needed before manage install
type manageSudoRequiredMsg struct {
	toolID string
}

// updateSudoRequiredMsg indicates sudo is needed before update
type updateSudoRequiredMsg struct {
	packages []pkg.Package
	all      bool
}

// manageStartInstallMsg triggers streaming install after sudo is cached
type manageStartInstallMsg struct {
	toolID string
}

// updateStartMsg triggers streaming update after sudo is cached
type updateStartMsg struct {
	packages []pkg.Package
	all      bool
}

// BackupEntry represents a backup in the list
type BackupEntry struct {
	Name      string
	Timestamp time.Time
	FileCount int
	Size      int64 // bytes
	Path      string
}

// backupsLoadedMsg indicates the async backup list loading completed
type backupsLoadedMsg struct {
	backups []BackupEntry
	err     error
}

// backupRestoreDoneMsg indicates a restore operation completed
type backupRestoreDoneMsg struct {
	name  string
	count int
	err   error
}

// backupDeleteDoneMsg indicates a delete operation completed
type backupDeleteDoneMsg struct {
	name string
	err  error
}

// backupCreateDoneMsg indicates a new backup was created
type backupCreateDoneMsg struct {
	name string
	err  error
}

// manageInstallWithLogsMsg carries install result with collected logs
type manageInstallWithLogsMsg struct {
	toolID string
	logs   []string
	err    error
}

// updateWithLogsMsg carries update result with collected logs
type updateWithLogsMsg struct {
	logs    []string
	results []pkg.UpdateResult
	err     error
}

// tickAnimation returns a command that sends tickMsg on each animation frame
func tickAnimation() tea.Cmd {
	return tea.Tick(introAnimationTick, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// tickUI returns a command that sends uiTickMsg for UI animations
func tickUI() tea.Cmd {
	return tea.Tick(uiTick, func(t time.Time) tea.Msg {
		return uiTickMsg(t)
	})
}

// checkDurdraw returns a command that checks if durdraw is available
func checkDurdraw() tea.Cmd {
	return func() tea.Msg {
		available := DetectDurdraw()
		return durdrawAvailableMsg(available)
	}
}
