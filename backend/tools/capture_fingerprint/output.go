package main

import (
	"crypto/md5"
	ctls "crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

func printCapture(c *Capture, outFile string) {
	data, _ := json.MarshalIndent(c, "", "  ")
	fmt.Println(string(data))
	if outFile != "" {
		if err := os.WriteFile(outFile, data, 0o644); err != nil {
			log.Printf("write %s: %v", outFile, err)
		} else {
			log.Printf("wrote fingerprint to %s", outFile)
		}
	}
}

func trimCRLF(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\r' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

func portOf(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}

func tlsVersionName(v uint16) string {
	switch v {
	case ctls.VersionTLS13:
		return "TLS 1.3"
	case ctls.VersionTLS12:
		return "TLS 1.2"
	}
	return fmt.Sprintf("0x%04x", v)
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}
