package main

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

type guardFileInfo struct {
	mode os.FileMode
	uid  uint32
}

func (info guardFileInfo) Name() string       { return "fixture" }
func (info guardFileInfo) Size() int64        { return 0 }
func (info guardFileInfo) Mode() os.FileMode  { return info.mode }
func (info guardFileInfo) ModTime() time.Time { return time.Time{} }
func (info guardFileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info guardFileInfo) Sys() any           { return &syscall.Stat_t{Uid: info.uid} }

func TestPreparationRefusesOwnerAndMutableInstallations(t *testing.T) {
	for _, failure := range []string{"none", "arch", "wrong-executable", "symlink", "writable", "foreign-owner", "missing-file", "bad-authorization"} {
		t.Run(failure, func(t *testing.T) {
			executable := qaExecutable
			if failure == "wrong-executable" {
				executable = "/tmp/platformqa"
			}
			lstat := func(path string) (os.FileInfo, error) {
				info := guardFileInfo{mode: 0555}
				if path == "/" || path == "/opt" || path == qaRoot {
					info.mode |= os.ModeDir
				}
				if path == qaRoot {
					switch failure {
					case "symlink":
						info.mode = os.ModeSymlink | 0777
					case "writable":
						info.mode |= 0020
					case "foreign-owner":
						info.uid = 501
					case "missing-file":
						return nil, os.ErrNotExist
					}
				}
				return info, nil
			}
			read := func(string) ([]byte, error) {
				if failure == "arch" {
					return []byte(qaArchAuthorization), nil
				}
				if failure == "bad-authorization" {
					return []byte("owner environment"), nil
				}
				return []byte(qaAuthorization), nil
			}
			err := checkPreparation(executable, lstat, read)
			if (err == nil) != (failure == "none" || failure == "arch") {
				t.Fatalf("guard=%v", err)
			}
		})
	}
	sentinel := errors.New("read failure")
	err := checkPreparation(qaExecutable, func(string) (os.FileInfo, error) { return nil, sentinel }, os.ReadFile)
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

func TestNormalRequestRequiresExplicitNonrootCI(t *testing.T) {
	for _, mode := range []string{"receipt", "receipt-held", "update", "upgrade"} {
		if !normalRequestAllowed(1001, "true", "true", "1", []string{mode}) {
			t.Fatal(mode)
		}
	}
	tests := []struct {
		uid               int
		ci, github, optin string
		args              []string
	}{
		{0, "true", "true", "1", []string{"update"}},
		{501, "", "true", "1", []string{"update"}},
		{501, "true", "", "1", []string{"update"}},
		{501, "true", "true", "", []string{"update"}},
		{501, "true", "true", "1", []string{"install", "arbitrary"}},
		{501, "true", "true", "1", nil},
	}
	for _, test := range tests {
		if normalRequestAllowed(test.uid, test.ci, test.github, test.optin, test.args) {
			t.Fatalf("accepted unsafe request: %+v", test)
		}
	}
}
