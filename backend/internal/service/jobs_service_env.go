package service

import "os"

func getenv(key string) string {
	return os.Getenv(key)
}
