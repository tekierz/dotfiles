package main

import "fmt"

func validateEffectiveUser(euid int) error {
	if euid != 0 {
		return nil
	}
	return fmt.Errorf("refusing to run dotfiles as root (effective UID 0): run it directly as the target user without sudo; root-owned configuration and backups cannot be safely restored by that user")
}
