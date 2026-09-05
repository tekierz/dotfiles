# Homebrew formula for dotfiles
# Install: brew install tekierz/tap/dotfiles
# This unified formula replaces dotfiles-setup and dotfiles-tui

class Dotfiles < Formula
  desc "Cross-platform terminal environment management TUI"
  homepage "https://github.com/tekierz/dotfiles"
  url "https://github.com/tekierz/dotfiles/archive/refs/tags/v2.0.1.tar.gz"
  sha256 "6b0991db618e24402a14029ef13e9db71a9a9d8d69313f1fa73de7b6d9d6986d"
  license "MIT"
  head "https://github.com/tekierz/dotfiles.git", branch: "main"

  depends_on "go" => :build

  def install
    # Clean up old binaries from previous installations
    old_binaries = ["dotfiles-tui", "dotfiles-setup"]
    old_binaries.each do |old_bin|
      old_path = HOMEBREW_PREFIX/"bin"/old_bin
      if old_path.exist?
        ohai "Removing old binary: #{old_bin}"
        old_path.unlink
      end
    end

    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/dotfiles"
  end

  def caveats
    <<~EOS
      Dotfiles has been installed!

      Commands:
        dotfiles              # Launch main menu
        dotfiles install      # Run installation wizard
        dotfiles manage       # Configure installed tools
        dotfiles hotkeys      # View keybindings cheatsheet
        dotfiles update       # Check for package updates
        dotfiles status       # Show current configuration
        dotfiles theme --list # List available themes
        dotfiles backups      # List configuration backups
        dotfiles restore <n>  # Restore a backup

      Features:
        - 16 color themes (neon-seapunk, catppuccin, dracula, etc.)
        - Interactive TUI with dual-pane configuration editor
        - Cross-platform (macOS, Arch, Debian)
        - Mouse and keyboard navigation
        - Streaming install logs
        - Batch package updates

      Note: This replaces the legacy dotfiles-setup and dotfiles-tui packages.
      If you had either installed, they have been automatically cleaned up.
    EOS
  end

  test do
    assert_match "dotfiles", shell_output("#{bin}/dotfiles --help")
  end
end
