class Dotfiles < Formula
  desc "Disposable release acceptance"
  homepage "https://github.com/tekierz/dotfiles"
  url "https://github.com/tekierz/dotfiles/archive/refs/tags/v2.0.1.tar.gz"
  version "2.0.1"
  sha256 "6b0991db618e24402a14029ef13e9db71a9a9d8d69313f1fa73de7b6d9d6986d"
  license "MIT"
  depends_on "go" => :build
  def install
    
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/dotfiles"
  end
  test do
    assert_match "dotfiles", shell_output("#{bin}/dotfiles --help")
  end
end
