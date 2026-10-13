package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	if name == "pg_dump" {
		_, _ = io.WriteString(os.Stdout, "CREATE TABLE sample (id integer);\nINSERT INTO sample VALUES (42);\n")
		return
	}
	for _, arg := range os.Args[1:] {
		if arg == "-c" {
			_, _ = fmt.Fprintln(os.Stdout, os.Getenv("FAKE_VAULT_FINGERPRINT"))
			return
		}
	}
	file, err := os.Create(os.Getenv("FAKE_RESTORE_OUTPUT"))
	if err != nil {
		os.Exit(1)
	}
	defer file.Close()
	if _, err = io.Copy(file, os.Stdin); err != nil {
		os.Exit(1)
	}
}
