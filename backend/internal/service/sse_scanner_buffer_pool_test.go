package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSEScannerBuf64KPool_GetPutDoesNotPanic(t *testing.T) {
	buf := getSSEScannerBuf64K()
	require.NotNil(t, buf)
	require.Equal(t, sseScannerBuf64KSize, len(buf[:]))

	buf[0] = 1
	putSSEScannerBuf64K(buf)

	// 允许传入 nil，确保不会 panic
	putSSEScannerBuf64K(nil)
}
