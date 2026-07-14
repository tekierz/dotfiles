//go:build darwin || linux

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRestoreSessionRestoreFileUsesImmutableSnapshotBytesAndMode(t *testing.T) {
	sourceRoot, source := restoreLeafFileSource(t, "accepted\n", 0o640)
	sourceDigest := source.Digest()
	mustWrite(t, filepath.Join(sourceRoot, "bundle/config"), "poisoned\n", 0o600)

	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "existing"}[existing], func(t *testing.T) {
			root := t.TempDir()
			if existing {
				mustWrite(t, filepath.Join(root, "config"), "installed\n", 0o600)
			}
			_, expected, parents, err := ObserveFileWithin(root, "config")
			requireRestoreLeaf(t, err == nil, "observe target: %v", err)
			revision, err := newRestoreSessionTest(t, root).RestoreFile("config", parents, expected, source, "config")
			requireRestoreLeaf(t, err == nil && revision.Exists() && revision.Permissions() == 0o640, "restore revision=%#v error=%v", revision, err)
			assertRestoreLeafFile(t, filepath.Join(root, "config"), "accepted\n", 0o640)
			requireRestoreLeaf(t, source.Digest() == sourceDigest, "source authority was mutated")
		})
	}
}

func TestRestoreSessionRestoreDirectoryUsesImmutableRecursiveSnapshot(t *testing.T) {
	sourceRoot, source := restoreLeafDirectorySource(t)
	sourceDigest := source.Digest()
	mustWrite(t, filepath.Join(sourceRoot, "bundle/tree/config"), "poisoned\n", 0o600)

	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "existing"}[existing], func(t *testing.T) {
			root := t.TempDir()
			if existing {
				mustMkdir(t, filepath.Join(root, "live"), 0o700)
				mustWrite(t, filepath.Join(root, "live/old"), "old\n", 0o600)
			}
			state, parents, err := ObserveDirectoryStateWithin(root, "live")
			requireRestoreLeaf(t, err == nil, "observe directory: %v", err)
			var tree *DirectorySnapshot
			if existing {
				tree, err = SnapshotDirectoryWithin(root, "live")
				requireRestoreLeaf(t, err == nil, "snapshot existing: %v", err)
			}
			evidence, err := newRestoreSessionTest(t, root).RestoreDirectory("live", parents, state, tree, source, "tree")
			requireRestoreLeaf(t, err == nil && evidence != nil && evidence.Permissions() == 0o750, "restore evidence=%#v error=%v", evidence, err)
			assertRestoreLeafFile(t, filepath.Join(root, "live/config"), "accepted\n", 0o640)
			assertRestoreDirTest(t, filepath.Join(root, "live/nested"), 0o710)
			assertRestoreLeafFile(t, filepath.Join(root, "live/nested/value"), "nested\n", 0o600)
			assertRestoreMissingTest(t, filepath.Join(root, "live/old"))
			requireRestoreLeaf(t, source.Digest() == sourceDigest, "source authority was mutated")
		})
	}
}

func TestRestoreSessionRemovesExactLeavesAndMissingLeavesWithoutCreatingParents(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "config"), "value", 0o600)
		_, expected, parents, err := ObserveFileWithin(root, "config")
		requireRestoreLeaf(t, err == nil, "observe: %v", err)
		err = newRestoreSessionTest(t, root).RemoveFile("config", parents, expected)
		requireRestoreLeaf(t, err == nil, "remove: %v", err)
		assertRestoreMissingTest(t, filepath.Join(root, "config"))
	})
	t.Run("directory", func(t *testing.T) {
		root := t.TempDir()
		mustMkdir(t, filepath.Join(root, "tree"), 0o750)
		mustWrite(t, filepath.Join(root, "tree/value"), "value", 0o600)
		state, parents, err := ObserveDirectoryStateWithin(root, "tree")
		requireRestoreLeaf(t, err == nil, "observe: %v", err)
		tree, err := SnapshotDirectoryWithin(root, "tree")
		requireRestoreLeaf(t, err == nil, "snapshot: %v", err)
		err = newRestoreSessionTest(t, root).RemoveDirectory("tree", parents, state, tree)
		requireRestoreLeaf(t, err == nil, "remove: %v", err)
		assertRestoreMissingTest(t, filepath.Join(root, "tree"))
	})
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-file", true: "missing-directory"}[directory], func(t *testing.T) {
			root := t.TempDir()
			session := newRestoreSessionTest(t, root)
			if directory {
				state, parents, err := ObserveDirectoryStateWithin(root, "missing/leaf")
				requireRestoreLeaf(t, err == nil && session.RemoveDirectory("missing/leaf", parents, state, nil) == nil, "missing directory remove: %v", err)
			} else {
				_, revision, parents, err := ObserveFileWithin(root, "missing/leaf")
				requireRestoreLeaf(t, err == nil && session.RemoveFile("missing/leaf", parents, revision) == nil, "missing file remove: %v", err)
			}
			assertRestoreMissingTest(t, filepath.Join(root, "missing"))
		})
	}
}

func TestRestoreSessionRejectsInvalidAuthorityAndSourceCombinations(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "existing"), 0o700)
	mustWrite(t, filepath.Join(root, "existing/value"), "accepted-tree", 0o640)
	mustWrite(t, filepath.Join(root, "file"), "accepted-file", 0o640)
	existingState, existingParents, _ := ObserveDirectoryStateWithin(root, "existing")
	existingTree, _ := SnapshotDirectoryWithin(root, "existing")
	sourceRoot, fileSource := restoreLeafFileSource(t, "value", 0o600)
	_, directorySource := restoreLeafDirectorySource(t)
	corruptSource := *fileSource
	corruptSource.digest[0] ^= 0xff
	_, missingFile, fileParents, _ := ObserveFileWithin(root, "file")
	missingDirectory, directoryParents, _ := ObserveDirectoryStateWithin(root, "directory")
	session := newRestoreSessionTest(t, root)
	var nilSession *RestoreSession
	foreignRoot := t.TempDir()
	_, _, foreignParents, _ := ObserveFileWithin(foreignRoot, "file")

	fileCalls := []func() error{
		func() error {
			return restoreLeafFileError(nilSession, "file", fileParents, missingFile, fileSource, "config")
		},
		func() error {
			return restoreLeafFileError(&RestoreSession{}, "file", fileParents, missingFile, fileSource, "config")
		},
		func() error {
			return restoreLeafFileError(session, "../file", fileParents, missingFile, fileSource, "config")
		},
		func() error { return restoreLeafFileError(session, "file", nil, missingFile, fileSource, "config") },
		func() error {
			return restoreLeafFileError(session, "file", fileParents, Revision{}, fileSource, "config")
		},
		func() error { return restoreLeafFileError(session, "file", fileParents, missingFile, nil, "config") },
		func() error {
			return restoreLeafFileError(session, "file", fileParents, missingFile, directorySource, "tree")
		},
		func() error {
			return restoreLeafFileError(session, "file", fileParents, missingFile, fileSource, "../config")
		},
		func() error {
			return restoreLeafFileError(session, "file", fileParents, missingFile, &corruptSource, "config")
		},
		func() error {
			return restoreLeafFileError(session, "file", foreignParents, missingFile, fileSource, "config")
		},
	}
	for index, call := range fileCalls {
		requireRestoreLeaf(t, call() != nil, "invalid file case %d was accepted", index)
	}
	directoryCalls := []func() error{
		func() error {
			return restoreLeafDirectoryError(session, "directory", directoryParents, DirectoryState{}, nil, directorySource, "tree")
		},
		func() error {
			return restoreLeafDirectoryError(session, "directory", directoryParents, missingDirectory, directorySource, directorySource, "tree")
		},
		func() error {
			return restoreLeafDirectoryError(session, "existing", existingParents, existingState, nil, directorySource, "tree")
		},
		func() error {
			return restoreLeafDirectoryError(session, "directory", nil, missingDirectory, nil, directorySource, "tree")
		},
		func() error {
			return restoreLeafDirectoryError(session, "directory", directoryParents, missingDirectory, nil, fileSource, "config")
		},
		func() error {
			return restoreLeafDirectoryError(nilSession, "directory", directoryParents, missingDirectory, nil, directorySource, "tree")
		},
		func() error {
			return restoreLeafDirectoryError(&RestoreSession{}, "directory", directoryParents, missingDirectory, nil, directorySource, "tree")
		},
		func() error {
			return restoreLeafDirectoryError(session, "../directory", directoryParents, missingDirectory, nil, directorySource, "tree")
		},
		func() error {
			return restoreLeafDirectoryError(session, "directory", foreignParents, missingDirectory, nil, directorySource, "tree")
		},
	}
	for index, call := range directoryCalls {
		requireRestoreLeaf(t, call() != nil, "invalid directory case %d was accepted", index)
	}
	assertRestoreLeafFile(t, filepath.Join(root, "file"), "accepted-file", 0o640)
	assertRestoreMissingTest(t, filepath.Join(root, "directory"))
	assertRestoreDirTest(t, filepath.Join(root, "existing"), 0o700)
	requireRestoreLeaf(t, fileSource.Digest() != ([32]byte{}) && sourceRoot != "", "source fixture invalid")
	removeCalls := []func() error{
		func() error { return nilSession.RemoveFile("file", fileParents, missingFile) },
		func() error { return (&RestoreSession{}).RemoveFile("file", fileParents, missingFile) },
		func() error { return session.RemoveFile("../file", fileParents, missingFile) },
		func() error { return session.RemoveFile("file", nil, missingFile) },
		func() error { return session.RemoveFile("file", fileParents, Revision{}) },
		func() error { return session.RemoveFile("file", foreignParents, missingFile) },
		func() error { return nilSession.RemoveDirectory("directory", directoryParents, missingDirectory, nil) },
		func() error {
			return (&RestoreSession{}).RemoveDirectory("directory", directoryParents, missingDirectory, nil)
		},
		func() error { return session.RemoveDirectory("../directory", directoryParents, missingDirectory, nil) },
		func() error { return session.RemoveDirectory("directory", nil, missingDirectory, nil) },
		func() error { return session.RemoveDirectory("directory", directoryParents, DirectoryState{}, nil) },
		func() error {
			return session.RemoveDirectory("directory", directoryParents, missingDirectory, directorySource)
		},
		func() error { return session.RemoveDirectory("existing", existingParents, existingState, nil) },
		func() error { return session.RemoveDirectory("directory", foreignParents, missingDirectory, nil) },
	}
	for index, call := range removeCalls {
		requireRestoreLeaf(t, call() != nil, "invalid remove case %d was accepted", index)
		data, currentFile, _, fileErr := ObserveFileWithin(root, "file")
		requireRestoreLeaf(t, fileErr == nil && currentFile == missingFile && string(data) == "accepted-file" && currentFile.Permissions() == 0o640,
			"invalid remove case %d changed accepted file identity/content: %v", index, fileErr)
		currentTree, treeErr := SnapshotDirectoryWithin(root, "existing")
		requireRestoreLeaf(t, treeErr == nil && SameDirectoryIdentity(existingTree, currentTree) && existingTree.Digest() == currentTree.Digest() && currentTree.Permissions() == 0o700,
			"invalid remove case %d changed accepted tree: %v", index, treeErr)
	}
}

func TestRestoreSessionRemoveRejectsExactLeafDrift(t *testing.T) {
	t.Run("file-replacement", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "config")
		mustWrite(t, path, "accepted", 0o600)
		_, expected, parents, _ := ObserveFileWithin(root, "config")
		requireRestoreLeaf(t, os.Remove(path) == nil, "remove accepted file")
		mustWrite(t, path, "foreign", 0o600)
		_, foreign, _, _ := ObserveFileWithin(root, "config")
		err := newRestoreSessionTest(t, root).RemoveFile("config", parents, expected)
		requireRestoreLeaf(t, err != nil, "replacement file was removed")
		data, after, _, readErr := ObserveFileWithin(root, "config")
		requireRestoreLeaf(t, readErr == nil && after == foreign && string(data) == "foreign" && after.Permissions() == 0o600, "foreign file changed: %v", readErr)
	})
	t.Run("directory-symlink", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "tree")
		mustMkdir(t, path, 0o700)
		state, parents, _ := ObserveDirectoryStateWithin(root, "tree")
		tree, _ := SnapshotDirectoryWithin(root, "tree")
		requireRestoreLeaf(t, os.Remove(path) == nil, "remove accepted directory")
		destination := t.TempDir()
		requireRestoreLeaf(t, os.Symlink(destination, path) == nil, "create replacement symlink")
		before, _ := os.Lstat(path)
		err := newRestoreSessionTest(t, root).RemoveDirectory("tree", parents, state, tree)
		requireRestoreLeaf(t, err != nil, "replacement symlink was removed")
		assertRestoreLeafSymlink(t, path, destination, before)
	})
}

func TestRestoreSessionRejectsNestedInvalidSourceBeforeParentCreation(t *testing.T) {
	_, source := restoreLeafFileSource(t, "value", 0o600)
	corrupt := *source
	corrupt.digest[0] ^= 0xff
	for _, test := range []struct {
		name, rel string
		source    *DirectorySnapshot
		directory bool
	}{
		{name: "corrupt-authority", rel: "config", source: &corrupt},
		{name: "invalid-descendant", rel: "missing", source: source},
		{name: "invalid-directory-descendant", rel: "missing", source: source, directory: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mkdirSentinel := errors.New("shallow mkdir must not run")
			mkdirCalls := 0
			directoryTestHooks = directoryHooks{
				beforeShallowMkdir: func(_ int, _ string) error { mkdirCalls++; return mkdirSentinel },
				afterShallowMkdir:  func(_, _ int, _ string) error { mkdirCalls++; return mkdirSentinel },
			}
			t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
			var err error
			if test.directory {
				state, parents, _ := ObserveDirectoryStateWithin(root, "new/parent/tree")
				_, err = newRestoreSessionTest(t, root).RestoreDirectory("new/parent/tree", parents, state, nil, test.source, test.rel)
			} else {
				_, expected, parents, _ := ObserveFileWithin(root, "new/parent/config")
				_, err = newRestoreSessionTest(t, root).RestoreFile("new/parent/config", parents, expected, test.source, test.rel)
			}
			requireRestoreLeaf(t, err != nil, "invalid nested source was accepted")
			requireRestoreLeaf(t, mkdirCalls == 0 && !errors.Is(err, mkdirSentinel), "invalid source entered shallow mkdir: calls=%d error=%v", mkdirCalls, err)
			assertRestoreMissingTest(t, filepath.Join(root, "new"))
			entries, readErr := os.ReadDir(root)
			requireRestoreLeaf(t, readErr == nil && len(entries) == 0, "rejected source left staging entries: %v %v", entries, readErr)
		})
	}
}

func TestRestoreSessionRejectsLeafAppearanceReplacementAndSymlinkBeforeMutation(t *testing.T) {
	_, source := restoreLeafFileSource(t, "accepted", 0o600)
	for _, mode := range []string{"appearance", "replacement", "type", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if mode != "appearance" {
				mustWrite(t, filepath.Join(root, "target"), "original", 0o600)
			}
			_, expected, parents, err := ObserveFileWithin(root, "target")
			requireRestoreLeaf(t, err == nil, "observe: %v", err)
			_ = os.Remove(filepath.Join(root, "target"))
			var foreignRevision Revision
			var symlinkBefore os.FileInfo
			switch mode {
			case "appearance", "replacement":
				mustWrite(t, filepath.Join(root, "target"), "foreign", 0o600)
				_, foreignRevision, _, _ = ObserveFileWithin(root, "target")
			case "type":
				mustMkdir(t, filepath.Join(root, "target"), 0o700)
			case "symlink":
				requireRestoreLeaf(t, os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "target")) == nil, "create symlink")
				symlinkBefore, _ = os.Lstat(filepath.Join(root, "target"))
			}
			_, err = newRestoreSessionTest(t, root).RestoreFile("target", parents, expected, source, "config")
			requireRestoreLeaf(t, err != nil, "%s drift was accepted", mode)
			if mode == "appearance" || mode == "replacement" {
				data, after, _, readErr := ObserveFileWithin(root, "target")
				requireRestoreLeaf(t, readErr == nil && after == foreignRevision && string(data) == "foreign" && after.Permissions() == 0o600, "foreign file changed: %v", readErr)
			} else if mode == "type" {
				assertRestoreDirTest(t, filepath.Join(root, "target"), 0o700)
			} else {
				assertRestoreLeafSymlink(t, filepath.Join(root, "target"), filepath.Join(root, "outside"), symlinkBefore)
			}
		})
	}
}

func TestRestoreSessionRejectsDirectoryAndNamespaceDrift(t *testing.T) {
	_, source := restoreLeafDirectorySource(t)
	t.Run("directory-replacement", func(t *testing.T) {
		root := t.TempDir()
		mustMkdir(t, filepath.Join(root, "live"), 0o700)
		mustWrite(t, filepath.Join(root, "live/value"), "original", 0o600)
		state, parents, _ := ObserveDirectoryStateWithin(root, "live")
		tree, _ := SnapshotDirectoryWithin(root, "live")
		requireRestoreLeaf(t, os.RemoveAll(filepath.Join(root, "live")) == nil, "remove old tree")
		mustMkdir(t, filepath.Join(root, "live"), 0o700)
		mustWrite(t, filepath.Join(root, "live/value"), "original", 0o600)
		foreign, _ := SnapshotDirectoryWithin(root, "live")
		_, err := newRestoreSessionTest(t, root).RestoreDirectory("live", parents, state, tree, source, "tree")
		requireRestoreLeaf(t, errors.Is(err, ErrDirectoryChanged), "replacement error=%v", err)
		assertRestoreLeafFile(t, filepath.Join(root, "live/value"), "original", 0o600)
		after, afterErr := SnapshotDirectoryWithin(root, "live")
		requireRestoreLeaf(t, afterErr == nil && SameDirectoryIdentity(foreign, after) && foreign.Digest() == after.Digest() && after.Permissions() == 0o700, "replacement directory identity/content changed: %v", afterErr)
	})
	t.Run("same-inode-recursive-drift", func(t *testing.T) {
		root := t.TempDir()
		mustMkdir(t, filepath.Join(root, "live"), 0o700)
		mustWrite(t, filepath.Join(root, "live/value"), "accepted", 0o600)
		state, parents, _ := ObserveDirectoryStateWithin(root, "live")
		tree, _ := SnapshotDirectoryWithin(root, "live")
		mustWrite(t, filepath.Join(root, "live/value"), "foreign", 0o600)
		foreignTree, _ := SnapshotDirectoryWithin(root, "live")
		_, err := newRestoreSessionTest(t, root).RestoreDirectory("live", parents, state, tree, source, "tree")
		requireRestoreLeaf(t, errors.Is(err, ErrDirectoryChanged), "recursive drift error=%v", err)
		after, afterErr := SnapshotDirectoryWithin(root, "live")
		requireRestoreLeaf(t, afterErr == nil && SameDirectoryIdentity(foreignTree, after) && foreignTree.Digest() == after.Digest() && after.Permissions() == 0o700,
			"same-inode foreign tree changed: %v", afterErr)
	})
	for _, kind := range []string{"file", "symlink"} {
		t.Run("appeared-"+kind, func(t *testing.T) {
			root := t.TempDir()
			state, parents, _ := ObserveDirectoryStateWithin(root, "live")
			foreign := ""
			var symlinkBefore os.FileInfo
			if kind == "file" {
				mustWrite(t, filepath.Join(root, "live"), "foreign", 0o600)
			} else {
				foreign = t.TempDir()
				requireRestoreLeaf(t, os.Symlink(foreign, filepath.Join(root, "live")) == nil, "create symlink")
				symlinkBefore, _ = os.Lstat(filepath.Join(root, "live"))
			}
			_, err := newRestoreSessionTest(t, root).RestoreDirectory("live", parents, state, nil, source, "tree")
			requireRestoreLeaf(t, err != nil, "appeared %s accepted", kind)
			if kind == "file" {
				assertRestoreLeafFile(t, filepath.Join(root, "live"), "foreign", 0o600)
			} else {
				assertRestoreLeafSymlink(t, filepath.Join(root, "live"), foreign, symlinkBefore)
			}
		})
	}
}

func TestRestoreSessionEveryLeafMethodRejectsParentAndRootSwap(t *testing.T) {
	for _, swap := range []string{"parent", "root"} {
		for _, method := range []string{"restore-file", "restore-directory", "remove-file", "remove-directory"} {
			t.Run(swap+"/"+method, func(t *testing.T) {
				workspace := t.TempDir()
				root := filepath.Join(workspace, "root")
				mustMkdir(t, root, 0o700)
				rel := "target"
				if swap == "parent" {
					mustMkdir(t, filepath.Join(root, "parent"), 0o700)
					rel = "parent/target"
				}
				operation, verifyAccepted := prepareRestoreLeafOperation(t, root, rel, method)
				session := newRestoreSessionTest(t, root)
				acceptedPath := filepath.Join(workspace, "moved", "target")
				if swap == "root" {
					requireRestoreLeaf(t, os.Rename(root, filepath.Join(workspace, "moved")) == nil, "move root")
					mustMkdir(t, root, 0o700)
				} else {
					requireRestoreLeaf(t, os.Rename(filepath.Join(root, "parent"), filepath.Join(root, "moved")) == nil, "move parent")
					mustMkdir(t, filepath.Join(root, "parent"), 0o700)
					acceptedPath = filepath.Join(root, "moved", "target")
				}
				marker := filepath.Join(root, filepath.Dir(rel), "foreign")
				mustWrite(t, marker, "keep", 0o600)
				requireRestoreLeaf(t, operation(session) != nil, "%s accepted %s swap", method, swap)
				assertRestoreLeafFile(t, marker, "keep", 0o600)
				verifyAccepted(acceptedPath)
			})
		}
	}
}

func prepareRestoreLeafOperation(t *testing.T, root, rel, method string) (func(*RestoreSession) error, func(string)) {
	t.Helper()
	if method == "restore-file" || method == "remove-file" {
		mustWrite(t, filepath.Join(root, rel), "accepted", 0o600)
		_, expected, parents, _ := ObserveFileWithin(root, rel)
		operation := func(session *RestoreSession) error {
			if method == "remove-file" {
				return session.RemoveFile(rel, parents, expected)
			}
			_, err := session.RestoreFile(rel, parents, expected, restoreLeafMustFileSource(t), "config")
			return err
		}
		verify := func(path string) {
			data, actual, _, err := ObserveFileWithin(filepath.Dir(path), filepath.Base(path))
			requireRestoreLeaf(t, err == nil && actual == expected && string(data) == "accepted" && actual.Permissions() == 0o600, "accepted file changed: %v", err)
		}
		return operation, verify
	}
	mustMkdir(t, filepath.Join(root, rel), 0o700)
	mustWrite(t, filepath.Join(root, rel, "value"), "accepted", 0o600)
	state, parents, _ := ObserveDirectoryStateWithin(root, rel)
	tree, _ := SnapshotDirectoryWithin(root, rel)
	operation := func(session *RestoreSession) error {
		if method == "remove-directory" {
			return session.RemoveDirectory(rel, parents, state, tree)
		}
		_, source := restoreLeafDirectorySource(t)
		_, err := session.RestoreDirectory(rel, parents, state, tree, source, "tree")
		return err
	}
	verify := func(path string) {
		actual, err := SnapshotDirectoryWithin(filepath.Dir(path), filepath.Base(path))
		requireRestoreLeaf(t, err == nil && SameDirectoryIdentity(tree, actual) && tree.Digest() == actual.Digest() && actual.Permissions() == 0o700,
			"accepted directory changed: %v", err)
	}
	return operation, verify
}

func TestRestoreSessionRollbackAndCommittedErrorPrecedence(t *testing.T) {
	primary := errors.New("injected leaf failure")
	for _, mode := range []string{"clean-rollback", "recovery", "committed"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			_, expected, parents, _ := ObserveFileWithin(root, "created/config")
			source := restoreLeafMustFileSource(t)
			replaceTestHooks = replaceHooks{}
			t.Cleanup(func() { replaceTestHooks = replaceHooks{} })
			if mode == "committed" {
				replaceTestHooks.afterCommit = func(_, _ int, _ string) error { return primary }
			} else {
				replaceTestHooks.beforeCommit = func(parentFD, _ int, _, _ string) error {
					if mode == "recovery" {
						_ = unix.Mkdirat(parentFD, "blocker", 0o700)
					}
					return primary
				}
			}
			_, err := newRestoreSessionTest(t, root).RestoreFile("created/config", parents, expected, source, "config")
			requireRestoreLeaf(t, errors.Is(err, primary), "%s error=%v", mode, err)
			var recovery *RecoveryError
			var committed *CommittedError
			switch mode {
			case "clean-rollback":
				requireRestoreLeaf(t, !errors.As(err, &recovery), "clean rollback became recovery: %v", err)
				assertRestoreMissingTest(t, filepath.Join(root, "created"))
			case "recovery":
				requireRestoreLeaf(t, errors.As(err, &recovery) && errors.Is(err, ErrDirectoryChanged), "recovery error=%v", err)
				assertRestoreDirTest(t, filepath.Join(root, "created/blocker"), 0o700)
			case "committed":
				requireRestoreLeaf(t, errors.As(err, &committed), "committed error=%v", err)
				assertRestoreLeafFile(t, filepath.Join(root, "created/config"), "value", 0o600)
			}
		})
	}
}

func TestRestoreSessionDirectoryRollbackAndRecoveryPrecedence(t *testing.T) {
	primary := errors.New("injected directory restore failure")
	for _, recovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "recovery"}[recovery], func(t *testing.T) {
			root := t.TempDir()
			state, parents, _ := ObserveDirectoryStateWithin(root, "created/live")
			_, source := restoreLeafDirectorySource(t)
			directoryTestHooks = directoryHooks{}
			t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
			directoryTestHooks.beforeInstall = func(parentFD, _ int, _, _ string) error {
				if recovery {
					_ = unix.Mkdirat(parentFD, "blocker", 0o700)
				}
				return primary
			}
			_, err := newRestoreSessionTest(t, root).RestoreDirectory("created/live", parents, state, nil, source, "tree")
			requireRestoreLeaf(t, errors.Is(err, primary), "directory rollback error=%v", err)
			var recoveryErr *RecoveryError
			if recovery {
				requireRestoreLeaf(t, errors.As(err, &recoveryErr) && errors.Is(err, ErrDirectoryChanged), "recovery precedence=%v", err)
				assertRestoreDirTest(t, filepath.Join(root, "created/blocker"), 0o700)
			} else {
				requireRestoreLeaf(t, !errors.As(err, &recoveryErr), "clean rollback became recovery: %v", err)
				assertRestoreMissingTest(t, filepath.Join(root, "created"))
			}
		})
	}
}

func TestRestoreSessionRemovalCommittedErrorsDoNotRollback(t *testing.T) {
	primary := errors.New("injected committed removal failure")
	t.Run("file", func(t *testing.T) {
		root := t.TempDir()
		mustMkdir(t, filepath.Join(root, "parent"), 0o700)
		mustWrite(t, filepath.Join(root, "parent/config"), "value", 0o600)
		_, expected, parents, _ := ObserveFileWithin(root, "parent/config")
		replaceTestHooks = replaceHooks{afterRemove: func(_, _ int, _ string) error { return primary }}
		t.Cleanup(func() { replaceTestHooks = replaceHooks{} })
		err := newRestoreSessionTest(t, root).RemoveFile("parent/config", parents, expected)
		var committed *CommittedError
		requireRestoreLeaf(t, errors.Is(err, primary) && errors.As(err, &committed), "file committed error=%v", err)
		assertRestoreMissingTest(t, filepath.Join(root, "parent/config"))
		assertRestoreDirTest(t, filepath.Join(root, "parent"), 0o700)
	})
	t.Run("directory", func(t *testing.T) {
		root := t.TempDir()
		mustMkdir(t, filepath.Join(root, "parent/tree"), 0o700)
		mustWrite(t, filepath.Join(root, "parent/tree/value"), "value", 0o600)
		state, parents, _ := ObserveDirectoryStateWithin(root, "parent/tree")
		tree, _ := SnapshotDirectoryWithin(root, "parent/tree")
		directoryTestHooks = directoryHooks{afterRemoveCommit: func(_ int, _ string) error { return primary }}
		t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
		err := newRestoreSessionTest(t, root).RemoveDirectory("parent/tree", parents, state, tree)
		var committed *CommittedError
		requireRestoreLeaf(t, errors.Is(err, primary) && errors.As(err, &committed), "directory committed error=%v", err)
		assertRestoreMissingTest(t, filepath.Join(root, "parent/tree"))
		assertRestoreDirTest(t, filepath.Join(root, "parent"), 0o700)
	})
}

func restoreLeafFileSource(t *testing.T, data string, mode os.FileMode) (string, *DirectorySnapshot) {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "bundle"), 0o700)
	mustWrite(t, filepath.Join(root, "bundle/config"), data, mode)
	snapshot, err := SnapshotDirectoryWithin(root, "bundle")
	requireRestoreLeaf(t, err == nil, "snapshot file source: %v", err)
	return root, snapshot
}

func restoreLeafMustFileSource(t *testing.T) *DirectorySnapshot {
	t.Helper()
	_, snapshot := restoreLeafFileSource(t, "value", 0o600)
	return snapshot
}

func restoreLeafDirectorySource(t *testing.T) (string, *DirectorySnapshot) {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "bundle/tree/nested"), 0o710)
	requireRestoreLeaf(t, os.Chmod(filepath.Join(root, "bundle/tree"), 0o750) == nil, "chmod source tree")
	mustWrite(t, filepath.Join(root, "bundle/tree/config"), "accepted\n", 0o640)
	mustWrite(t, filepath.Join(root, "bundle/tree/nested/value"), "nested\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "bundle")
	requireRestoreLeaf(t, err == nil, "snapshot directory source: %v", err)
	return root, snapshot
}

func assertRestoreLeafFile(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	actual, err := os.ReadFile(path)
	info, statErr := os.Lstat(path)
	requireRestoreLeaf(t, err == nil && statErr == nil && info.Mode().IsRegular() && info.Mode().Perm() == mode.Perm() && string(actual) == data,
		"file %q data=%q mode=%v read=%v stat=%v", path, actual, infoMode(info), err, statErr)
}

func assertRestoreLeafSymlink(t *testing.T, path, target string, before os.FileInfo) {
	t.Helper()
	after, statErr := os.Lstat(path)
	destination, readErr := os.Readlink(path)
	requireRestoreLeaf(t, before != nil && statErr == nil && readErr == nil && os.SameFile(before, after) && before.Mode() == after.Mode() && destination == target,
		"symlink %q changed: target=%q mode=%v stat=%v read=%v", path, destination, infoMode(after), statErr, readErr)
}

func restoreLeafFileError(session *RestoreSession, rel string, parents *ParentChain, expected Revision, source *DirectorySnapshot, sourceRel string) error {
	_, err := session.RestoreFile(rel, parents, expected, source, sourceRel)
	return err
}

func restoreLeafDirectoryError(session *RestoreSession, rel string, parents *ParentChain, state DirectoryState, tree, source *DirectorySnapshot, sourceRel string) error {
	_, err := session.RestoreDirectory(rel, parents, state, tree, source, sourceRel)
	return err
}

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode()
}

func requireRestoreLeaf(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()
	if !condition {
		t.Fatalf(format, args...)
	}
}
