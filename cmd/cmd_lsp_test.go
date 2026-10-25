package cmd

import (
	"fmt"
	"strings"
	"testing"
)

// TestLSPCodeComments checks that the lsp command reads source comments by default and leaves
// source files alone with --code-comments=false, while prose is checked either way.
func TestLSPCodeComments(t *testing.T) {
	tests := []struct {
		In           []string
		WantSource   bool
		WantProseHit bool
	}{{ // Test 0: By default a source comment is checked.
		In: []string{"lsp"}, WantSource: true, WantProseHit: true,
	}, { // Test 1: Off, the source file gets nothing and prose is still checked.
		In: []string{"lsp", "--code-comments=false"}, WantSource: false, WantProseHit: true,
	}}
	var in strings.Builder
	for _, body := range []string{
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":` +
			`{"textDocument":{"uri":"file:///main.go","text":"// we leverage it\n"}}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":` +
			`{"textDocument":{"uri":"file:///notes.md","text":"we leverage it"}}}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	} {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			out, _, err := runCLI(t, test.In, in.String())
			if err != nil {
				t.Fatalf("lsp: %v", err)
			}
			source, prose, ok := strings.Cut(out, `"uri":"file:///notes.md"`)
			if !ok {
				t.Fatalf("no diagnostics for the prose file in %s", out)
			}
			if got := strings.Contains(source, "leverage"); got != test.WantSource {
				t.Errorf("source comment checked = %v, want %v", got, test.WantSource)
			}
			if got := strings.Contains(prose, "leverage"); got != test.WantProseHit {
				t.Errorf("prose checked = %v, want %v", got, test.WantProseHit)
			}
		})
	}
}
