//go:build !darwin && !linux

package main

func effectiveUserID() int {
	return -1
}
