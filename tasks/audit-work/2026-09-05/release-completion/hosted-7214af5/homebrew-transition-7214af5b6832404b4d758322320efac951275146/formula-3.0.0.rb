class Dotfiles < Formula
  desc "Disposable release acceptance"
  homepage "https://github.com/tekierz/dotfiles"
  url "https://github.com/tekierz/dotfiles/archive/7214af5b6832404b4d758322320efac951275146.tar.gz"
  version "3.0.0"
  sha256 "429d86a03b09a519a204ba220101bb6b9549e44aba56e51bb6f55f21ded66eb4"
  license "MIT"
  depends_on "go" => :build
  def install
    ENV["GOTOOLCHAIN"] = "go1.26.8"
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/dotfiles"
  end
  test do
    assert_match "dotfiles", shell_output("#{bin}/dotfiles --help")
  end
end
