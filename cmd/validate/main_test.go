package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunArgumentsAndDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		out  string
		err  string
	}{
		{"embedded content", nil, 0, "OK\n", ""},
		{"version", []string{"--version"}, 0, "tower-validate dev\n", ""},
		{"unknown flag", []string{"--bad"}, 2, "", "usage: validate"},
		{"too many paths", []string{"one", "two"}, 2, "", "usage: validate"},
		{"missing path", []string{"/no/such/asset-directory"}, 1, "reading asset directory", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.code {
				t.Fatalf("exit = %d, want %d; stdout=%q stderr=%q", got, tt.code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.out) || !strings.Contains(stderr.String(), tt.err) {
				t.Errorf("stdout=%q stderr=%q; want substrings %q / %q", stdout.String(), stderr.String(), tt.out, tt.err)
			}
		})
	}
}
