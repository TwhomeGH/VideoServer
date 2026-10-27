//go:build !windows

package library

func isReparsePoint(string) bool   { return false }
func isHiddenOrSystem(string) bool { return false }
