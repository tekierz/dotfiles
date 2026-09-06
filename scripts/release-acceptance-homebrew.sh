#!/usr/bin/env bash
# Real version transitions, exclusively on a disposable hosted macOS runner.
set -euo pipefail
if [[ ${GITHUB_ACTIONS:-} != true || ${RUNNER_ENVIRONMENT:-} != github-hosted ||
      ${RUNNER_OS:-} != macOS || ${DOTFILES_RELEASE_ACCEPTANCE:-} != 1 || $(uname -s) != Darwin ]]; then
  echo 'Homebrew release acceptance requires its disposable hosted macOS job' >&2
  exit 125
fi
: "${RUNNER_TEMP:?}"
: "${GITHUB_SHA:?}"
: "${RELEASE_VERSION:?}"
[[ $GITHUB_SHA =~ ^[a-f0-9]{40}$ ]]
[[ $RELEASE_VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && $RELEASE_VERSION != 2.0.1 ]]
export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1
export HOMEBREW_NO_AUTOREMOVE=1 HOMEBREW_NO_ANALYTICS=1
trial_prefix=$(brew --prefix)
case "$(uname -m):$trial_prefix" in
  arm64:/opt/homebrew|x86_64:/usr/local) ;;
  *) exit 125 ;;
esac
trial_name=release/acceptance/dotfiles
trial_tap_dir="$(brew --repository)/Library/Taps/release/homebrew-acceptance"
trial_cellar="$(brew --cellar)/dotfiles"
for path in "$trial_prefix/bin/dotfiles" "$trial_prefix/opt/dotfiles" "$trial_cellar" "$trial_tap_dir"; do
  if [[ -e $path || -L $path ]]; then
    echo "Preexisting acceptance target: $path" >&2
    exit 125
  fi
done
trial_root=$(mktemp -d "$RUNNER_TEMP/dotfiles-release.XXXXXX")
trial_home="$trial_root/home"
trial_evidence="$RUNNER_TEMP/dotfiles-homebrew-evidence"
[[ ! -e $trial_evidence ]]
mkdir -m 700 "$trial_home" "$trial_evidence"
mkdir -p "$trial_home/.config/ghostty" "$trial_home/.config/dotfiles/backups/recovery-fixture"
printf '# preserved shell configuration\n' > "$trial_home/.zshrc"
printf 'font-size = 13\n' > "$trial_home/.config/ghostty/config"
printf 'independent recovery bytes\n' > "$trial_home/.config/dotfiles/backups/recovery-fixture/sentinel"
chmod 600 "$trial_home/.zshrc" "$trial_home/.config/dotfiles/backups/recovery-fixture/sentinel"
chmod 640 "$trial_home/.config/ghostty/config"
chmod 700 "$trial_home/.config" "$trial_home/.config/dotfiles" "$trial_home/.config/dotfiles/backups"
cat > "$trial_root/snapshot.rb" <<'RUBY'
require "json"
require "digest"
root = ARGV.fetch(0)
paths = [".zshrc", ".config/ghostty", ".config/dotfiles/backups"].flat_map { |rel| p = File.join(root, rel); [p] + (File.directory?(p) ? Dir.glob("#{p}/**/*", File::FNM_DOTMATCH) : []) }
result = paths.sort.uniq.reject { |p| [".", ".."].include?(File.basename(p)) }.to_h do |path|
  stat = File.lstat(path)
  data = {type: stat.ftype, mode: stat.mode & 07777}
  data[:sha256] = Digest::SHA256.file(path).hexdigest if stat.file?
  data[:target] = File.readlink(path) if stat.symlink?
  [path.delete_prefix("#{root}/"), data]
end
puts JSON.pretty_generate(result)
RUBY
export HOME="$trial_home" XDG_CONFIG_HOME="$trial_home/.config"
export XDG_STATE_HOME="$trial_home/.local/state" XDG_CACHE_HOME="$trial_root/cache"
export HOMEBREW_CACHE="$trial_root/brew-cache" HOMEBREW_LOGS="$trial_root/brew-logs"
export HOMEBREW_TEMP="$trial_root/brew-temp"
mkdir -p "$HOMEBREW_CACHE" "$HOMEBREW_LOGS" "$HOMEBREW_TEMP" "$XDG_CACHE_HOME"
ruby "$trial_root/snapshot.rb" "$trial_home" > "$trial_evidence/config-before.json"
preserved() {
  ruby "$trial_root/snapshot.rb" "$trial_home" > "$trial_evidence/config-$1.json"
  cmp "$trial_evidence/config-before.json" "$trial_evidence/config-$1.json"
}
run_app() {
  env HOME="$trial_home" XDG_CONFIG_HOME="$trial_home/.config" \
    XDG_STATE_HOME="$trial_home/.local/state" XDG_CACHE_HOME="$trial_home/.cache" "$trial_prefix/bin/dotfiles" "$@"
}
old_url=https://github.com/tekierz/dotfiles/archive/refs/tags/v2.0.1.tar.gz
old_hash=6b0991db618e24402a14029ef13e9db71a9a9d8d69313f1fa73de7b6d9d6986d
candidate_url="https://github.com/tekierz/dotfiles/archive/${GITHUB_SHA}.tar.gz"
curl --fail --location --retry 3 "$candidate_url" -o "$trial_root/candidate.tar.gz"
candidate_hash=$(shasum -a 256 "$trial_root/candidate.tar.gz" | awk '{print $1}')
printf 'candidate_commit=%s\ncandidate_version=%s\ncandidate_url=%s\ncandidate_sha256=%s\nold_version=2.0.1\nold_url=%s\nold_sha256=%s\n' \
  "$GITHUB_SHA" "$RELEASE_VERSION" "$candidate_url" "$candidate_hash" "$old_url" "$old_hash" > "$trial_evidence/sources.txt"
brew --version > "$trial_evidence/homebrew-version.txt"
uname -a > "$trial_evidence/platform.txt"
brew tap-new --no-git release/acceptance
write_formula() {
  local version=$1 url=$2 hash=$3 toolchain_line=''
  if [[ $version == "$RELEASE_VERSION" ]]; then
    toolchain_line='ENV["GOTOOLCHAIN"] = "go1.26.8"'
  fi
  cat > "$trial_tap_dir/Formula/dotfiles.rb" <<RUBY
class Dotfiles < Formula
  desc "Disposable release acceptance"
  homepage "https://github.com/tekierz/dotfiles"
  url "$url"
  version "$version"
  sha256 "$hash"
  license "MIT"
  depends_on "go" => :build
  def install
    $toolchain_line
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/dotfiles"
  end
  test do
    assert_match "dotfiles", shell_output("#{bin}/dotfiles --help")
  end
end
RUBY
  cp "$trial_tap_dir/Formula/dotfiles.rb" "$trial_evidence/formula-$version.rb"
}
record_version() {
  local phase=$1 expected=$2
  [[ $(run_app version) == "dotfiles version $expected" ]]
  run_app version > "$trial_evidence/$phase-version.txt"
  run_app --help > "$trial_evidence/$phase-help.txt"
  brew list --versions "$trial_name" > "$trial_evidence/$phase-kegs.txt"
  shasum -a 256 "$trial_prefix/bin/dotfiles" > "$trial_evidence/$phase-binary.sha256"
  go version -m "$trial_prefix/bin/dotfiles" > "$trial_evidence/$phase-build.txt"
  if [[ $expected == "$RELEASE_VERSION" ]]; then
    head -n 1 "$trial_evidence/$phase-build.txt" | grep -F 'go1.26.8'
  fi
  cp "$trial_cellar/$expected/INSTALL_RECEIPT.json" "$trial_evidence/$phase-receipt.json"
  preserved "$phase"
}

# Clean installation of exactly the proposed source and version.
write_formula "$RELEASE_VERSION" "$candidate_url" "$candidate_hash"
brew install --build-from-source "$trial_name"
brew test "$trial_name"
record_version clean "$RELEASE_VERSION"
[[ $(run_app --version) == "dotfiles version $RELEASE_VERSION" ]]
brew uninstall "$trial_name"
[[ ! -e $trial_prefix/bin/dotfiles && ! -L $trial_prefix/bin/dotfiles ]]
preserved clean-uninstall

# Retain the actual older keg; rollback must recover these exact binary bytes.
write_formula 2.0.1 "$old_url" "$old_hash"
brew install --build-from-source "$trial_name"
record_version old 2.0.1
old_binary_hash=$(shasum -a 256 "$trial_cellar/2.0.1/bin/dotfiles" | awk '{print $1}')
write_formula "$RELEASE_VERSION" "$candidate_url" "$candidate_hash"
# A named outdated formula deliberately returns 1. Validate the expected
# receipt/version payload separately so an operational failure cannot pass.
set +e
brew outdated --json=v2 "$trial_name" > "$trial_evidence/outdated.json"
outdated_exit=$?
set -e
[[ $outdated_exit == 1 ]]
ruby -rjson -e 'j=JSON.parse(File.read(ARGV[0])); abort "missing real upgrade" unless j.fetch("formulae").any? { |f| f.fetch("installed_versions").include?("2.0.1") && f.fetch("current_version") == ARGV[1] }' \
  "$trial_evidence/outdated.json" "$RELEASE_VERSION"
brew upgrade --build-from-source "$trial_name"
record_version upgraded "$RELEASE_VERSION"
[[ -x $trial_cellar/2.0.1/bin/dotfiles ]]
[[ $(shasum -a 256 "$trial_cellar/2.0.1/bin/dotfiles" | awk '{print $1}') == "$old_binary_hash" ]]

# Homebrew removes the currently linked candidate keg. It retains the old keg
# because --force is absent, allowing an ordinary manager-owned relink.
brew uninstall "$trial_name"
[[ ! -e $trial_cellar/$RELEASE_VERSION && -x $trial_cellar/2.0.1/bin/dotfiles ]]
write_formula 2.0.1 "$old_url" "$old_hash"
brew link "$trial_name"
record_version rollback 2.0.1
[[ $(shasum -a 256 "$trial_prefix/bin/dotfiles" | awk '{print $1}') == "$old_binary_hash" ]]
brew test "$trial_name"
brew uninstall "$trial_name"
preserved final-uninstall
printf 'DOTFILES_HOMEBREW_RELEASE_ACCEPTANCE_PASSED clean=%s upgrade=2.0.1-to-%s rollback=2.0.1 config=bytes-and-modes source=%s\n' \
  "$RELEASE_VERSION" "$RELEASE_VERSION" "$GITHUB_SHA" | tee "$trial_evidence/result.txt"
# Preserve the local tap and fixtures for inspection; VM teardown owns cleanup.
