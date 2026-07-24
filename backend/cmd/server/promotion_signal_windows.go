//go:build windows

package main

import "os"

func notifyPromotionSignal(_ chan<- os.Signal) {}
