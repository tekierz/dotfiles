# Homebrew ownership fix proposal

Prepared against `tekierz/homebrew-tap` commit `74b68dc`. The patch removes basename-only deletion of legacy binaries and corrects migration guidance. Existing v2.0.1 source URL/version/hash remain unchanged: downloaded archive SHA-256 verified as `6b0991db618e24402a14029ef13e9db71a9a9d8d69313f1fa73de7b6d9d6986d` on 2026-09-05.

Apply with `git apply --unidiff-zero ownership-fix.patch` in a tap checkout and copy the Ruby test to `test/dotfiles_ownership_test.rb`. Run `ruby test/dotfiles_ownership_test.rb`. The test stubs only Homebrew/build DSL calls and executes the actual formula install method against temporary regular files, symlinks and dangling symlinks; it does not build/install packages. Current formula failed by removing `dotfiles-tui`; patched formula passed 15 assertions. Ruby syntax and whitespace checks passed.

No tap branch was pushed or merged, and no Homebrew install/upgrade was run on the owner machine. A real isolated Homebrew clean-install/upgrade/rollback trial and final-release artifact update remain acceptance gates. This patch can fix legacy-file ownership independently of the next product release; it does not distribute the integration candidate.
