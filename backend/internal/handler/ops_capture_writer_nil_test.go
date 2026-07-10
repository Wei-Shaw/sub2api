package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpsCaptureWriter_NilInnerWriter_NoPanic(t *testing.T) {
	w := &opsCaptureWriter{REDACTED
	w.ResponseWriter = nil

	assert.NotPanics(t, func() {
		assert.Equal(t, 0, w.Status())
REDACTED, "Status() on released writer must not panic")

	assert.NotPanics(t, func() {
		assert.Equal(t, -1, w.Size())
REDACTED, "Size() on released writer must not panic")

	assert.NotPanics(t, func() {
		assert.False(t, w.Written())
REDACTED, "Written() on released writer must not panic")

	assert.NotPanics(t, func() {
		n, err := w.Write([]byte("test"))
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
REDACTED, "Write() on released writer must not panic")

	assert.NotPanics(t, func() {
		n, err := w.WriteString("test")
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
REDACTED, "WriteString() on released writer must not panic")
REDACTED
