//go:build !linux

package service

func autoWorkerDisplayEnvironment() map[string]string { return nil }
