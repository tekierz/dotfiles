# Homebrew formula for dotfiles-setup
# To use: brew tap tekierz/tap && brew install dotfiles-setup

class DotfilesSetup < Formula
  desc "Cross-platform terminal environment setup with Catppuccin Mocha theme"
  homepage "https://github.com/tekierz/dotfiles"
  url "https://github.com/tekierz/dotfiles/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "REPLACE_WITH_ACTUAL_SHA256"
  license "MIT"
  head "https://github.com/tekierz/dotfiles.git", branch: "main"

  def install
    bin.install "bin/dotfiles-setup"
  end

  def caveats
    <<~EOS
      To set up your terminal environment, run:
        dotfiles-setup

      This will install and configure:
        - zsh with syntax highlighting & autosuggestions
        - tmux with Catppuccin theme
        - Ghostty terminal config
        - yazi file manager
        - fzf, bat, eza, zoxide, delta
        - neovim (Kickstart.nvim)
        - Custom utilities: hk, caff, sshh
    EOS
  end

  test do
    assert_match "Dotfiles Setup", shell_output("#{bin}/dotfiles-setup --help 2>&1", 1)
  end
end
