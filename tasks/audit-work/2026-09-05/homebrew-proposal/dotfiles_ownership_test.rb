# Run with ruby test/dotfiles_ownership_test.rb. No package build or install runs.
require "minitest/autorun"
require "pathname"
require "tmpdir"
require "fileutils"

class Formula
  def self.method_missing(*) = nil
  def self.respond_to_missing?(*) = true
  def self.test(*) = nil
  def std_go_args(**) = ["-o", "isolated-keg/bin/dotfiles"]
  def version = "2.0.1"
  def ohai(*) = nil
  attr_reader :build_command
  def system(*args)
    @build_command = args
    true
  end
end

load ENV.fetch("DOTFILES_FORMULA", File.expand_path("../Formula/dotfiles.rb", __dir__))

class DotfilesOwnershipTest < Minitest::Test
  def test_install_preserves_unowned_legacy_files_and_symlinks
    [:file, :symlink, :dangling_symlink].each do |kind|
      Dir.mktmpdir("formula-ownership") do |dir|
        prefix = Pathname.new(dir)
        FileUtils.mkdir_p(prefix/"bin")
        Object.const_set(:HOMEBREW_PREFIX, prefix)
        paths = %w[dotfiles-tui dotfiles-setup].map { |name| prefix/"bin"/name }
        paths.each do |path|
          case kind
          when :file then path.write("user-owned #{path.basename}\n")
          when :symlink
            target = prefix/"owned-#{path.basename}"
            target.write("external owner\n")
            File.symlink(target, path)
          when :dangling_symlink then File.symlink(prefix/"missing-#{path.basename}", path)
          end
        end
        before = paths.map { |path| path.symlink? ? [:symlink, path.readlink.to_s] : [:file, path.read] }
        formula = Dotfiles.new
        formula.install
        paths.zip(before).each do |path, (type, content)|
          if type == :symlink
            assert path.symlink?, "removed or replaced #{kind}: #{path.basename}"
            assert_equal content, path.readlink.to_s
          else
            assert path.file?, "removed #{path.basename}"
            assert_equal content, path.read
          end
        end
        assert_equal ["go", "build", "-o", "isolated-keg/bin/dotfiles", "./cmd/dotfiles"], formula.build_command
      ensure
        Object.send(:remove_const, :HOMEBREW_PREFIX) if Object.const_defined?(:HOMEBREW_PREFIX)
      end
    end
  end
end
