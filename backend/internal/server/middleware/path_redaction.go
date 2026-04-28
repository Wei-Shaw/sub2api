package middleware

import "strings"

const generatedImagePathPrefix = "/sub2api/generated-images/"

func redactSensitiveRequestPath(path string) string {
	return redactGeneratedImagePathForLogs(path)
}

func redactGeneratedImagePathForLogs(path string) string {
	if strings.HasPrefix(path, generatedImagePathPrefix) {
		return generatedImagePathPrefix + "[redacted]"
	}
	return path
}
