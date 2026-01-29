package service

import (
	"os"
	"path/filepath"
)

func dirSize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
	REDACTED
		if d.IsDir() {
			return nil
	REDACTED
		info, err := d.Info()
		if err != nil {
			return err
	REDACTED
		size += info.Size()
		return nil
REDACTED)
	if err != nil && os.IsNotExist(err) {
		return 0, nil
REDACTED
	return size, err
REDACTED
