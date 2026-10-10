// Package util holds small shared helpers: password generation, free-port
// probing, and clipboard copying.
package util

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GeneratePassword returns a random alphanumeric password of length n.
func GeneratePassword(n int) string {
	if n <= 0 {
		n = 16
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "changeme12345678"[:n]
	}
	for i := range b {
		b[i] = passwordChars[int(b[i])%len(passwordChars)]
	}
	return string(b)
}

// FreePort reports whether localhost:port is free to bind.
func FreePort(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// FindFreePort scans upward from start for a free port.
func FindFreePort(start int) int {
	for p := start; p < start+10000; p++ {
		if FreePort(p) {
			return p
		}
	}
	return start
}

// CopyToClipboard tries OSC52, then wl-copy / xclip / xsel / pbcopy.
func CopyToClipboard(s string) error {
	// OSC52 works over SSH and most modern terminals; harmless if unsupported.
	fmt.Fprintf(os.Stdout, "\033]52;c;%s\a", b64(s))
	for _, c := range []struct {
		bin  string
		args []string
	}{{"wl-copy", nil}, {"xclip", []string{"-selection", "clipboard"}}, {"xsel", []string{"--clipboard", "--input"}}, {"pbcopy", nil}} {
		if _, err := exec.LookPath(c.bin); err == nil {
			cmd := exec.Command(c.bin, c.args...)
			cmd.Stdin = strings.NewReader(s)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}
	return nil
}
