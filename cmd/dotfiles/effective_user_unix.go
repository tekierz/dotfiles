//go:build darwin || linux

package main

import "os"

func effectiveUserID() int {
	return os.Geteuid()
}
