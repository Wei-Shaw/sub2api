//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func notifyPromotionSignal(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGUSR1)
}
