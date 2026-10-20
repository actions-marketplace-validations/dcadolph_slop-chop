package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/slop-chop/sanitize"
)

// TestServer drives the server over a scripted message stream and checks the initialize
// result, the diagnostics from an opened document, and the formatting edit that chops it.
func TestServer(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{
		WordReplace:    map[string]string{"leverage": "use"},
		CollapseSpaces: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var in strings.Builder
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":` +
			`{"textDocument":{"uri":"file:///a","text":"we leverage it"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":` +
			`{"textDocument":{"uri":"file:///a"}}}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	} {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}

	var out bytes.Buffer
	if err := NewServer(san, strings.NewReader(in.String()), &out).Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	frames := splitFrames(t, out.Bytes())
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3", len(frames))
	}

	// Test 0: initialize advertises the server name and its capabilities.
	var init struct {
		Result initializeResult `json:"result"`
	}
	mustUnmarshal(t, frames[0], &init)
	if init.Result.ServerInfo.Name != "slop-chop" {
		t.Errorf("server name = %q, want slop-chop", init.Result.ServerInfo.Name)
	}
	if !init.Result.Capabilities.DocumentFormattingProvider || !init.Result.Capabilities.CodeActionProvider {
		t.Errorf("capabilities = %+v, want formatting and code actions", init.Result.Capabilities)
	}

	// Test 1: opening a document publishes a diagnostic for the tell at the right span.
	var diag struct {
		Method string                   `json:"method"`
		Params publishDiagnosticsParams `json:"params"`
	}
	mustUnmarshal(t, frames[1], &diag)
	if diag.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("method = %q, want publishDiagnostics", diag.Method)
	}
	if len(diag.Params.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1", len(diag.Params.Diagnostics))
	}
	d := diag.Params.Diagnostics[0]
	if d.Source != "slop-chop" || d.Range.Start.Character != 3 || d.Range.End.Character != 11 {
		t.Errorf("diagnostic = %+v, want source slop-chop at 3..11", d)
	}

	// Test 2: formatting returns the whole document chopped.
	var format struct {
		Result []TextEdit `json:"result"`
	}
	mustUnmarshal(t, frames[2], &format)
	if len(format.Result) != 1 || format.Result[0].NewText != "we use it" {
		t.Errorf("format result = %+v, want one edit to \"we use it\"", format.Result)
	}
}

// TestPublishOverlappingFindings checks that a finding nested inside a wider one keeps its
// own diagnostic range. The forward-only position walk must be fed sorted offsets, or the
// inner finding's start, which sits behind the outer finding's end, collapses to a point.
func TestPublishOverlappingFindings(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{
		BlockWords:   []string{"powerful"},
		FlagPatterns: map[string]string{"neg-parallel": `not just .+ but also`},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var in strings.Builder
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":` +
			`{"textDocument":{"uri":"file:///a","text":"not just powerful, but also fast"}}}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	} {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}

	var out bytes.Buffer
	if err := NewServer(san, strings.NewReader(in.String()), &out).Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	frames := splitFrames(t, out.Bytes())

	var diag struct {
		Params publishDiagnosticsParams `json:"params"`
	}
	mustUnmarshal(t, frames[1], &diag)

	// The block word sits inside the wider structural span, so its start is behind that
	// span's end. Its diagnostic must still cover "powerful" at characters 9..17.
	var block *Diagnostic
	for i := range diag.Params.Diagnostics {
		if diag.Params.Diagnostics[i].Code == "word:powerful" {
			block = &diag.Params.Diagnostics[i]
		}
	}
	if block == nil {
		t.Fatalf("no powerful diagnostic in %+v", diag.Params.Diagnostics)
	}
	if block.Range.Start.Character != 9 || block.Range.End.Character != 17 {
		t.Errorf("powerful range = %d..%d, want 9..17",
			block.Range.Start.Character, block.Range.End.Character)
	}
}

// TestReadMessageHardening checks the framing against hostile and spec-legal headers: a
// lowercase header is accepted, and an absurd Content-Length is a clean error, not a panic.
func TestReadMessageHardening(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Test 0: a lowercase content-length header frames the message fine.
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	in := fmt.Sprintf("content-length: %d\r\n\r\n%s", len(body), body)
	var out bytes.Buffer
	if err := NewServer(san, strings.NewReader(in), &out).Run(); err != nil {
		t.Errorf("lowercase header: Run = %v, want clean EOF end", err)
	}
	if got := splitFrames(t, out.Bytes()); len(got) != 1 {
		t.Errorf("lowercase header: frames = %d, want the initialize reply", len(got))
	}

	// Test 1: a giant Content-Length is refused as an error instead of panicking makeslice.
	huge := "Content-Length: 9223372036854775807\r\n\r\n"
	err = NewServer(san, strings.NewReader(huge), &bytes.Buffer{}).Run()
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("huge length: err = %v, want a too-large error", err)
	}
}

// TestPositionMapMatchesScan checks the single-pass position map against the reference
// scanner on multi-byte text, so the fast path cannot drift from the correct one.
func TestPositionMapMatchesScan(t *testing.T) {
	t.Parallel()
	text := "café — plain\nnew 😀 line\nlast"
	pos := newPositionMap(text)
	for off := 0; off <= len(text); off++ {
		want := offsetToPosition(text, off)
		got := pos.at(off)
		if got != want {
			t.Fatalf("offset %d: got %+v, want %+v", off, got, want)
		}
	}
}

// splitFrames breaks a Content-Length framed stream into its message bodies.
func splitFrames(t *testing.T, b []byte) [][]byte {
	t.Helper()
	var out [][]byte
	for len(b) > 0 {
		sep := bytes.Index(b, []byte("\r\n\r\n"))
		if sep < 0 {
			break
		}
		var n int
		for _, line := range strings.Split(string(b[:sep]), "\r\n") {
			if after, ok := strings.CutPrefix(line, "Content-Length:"); ok {
				n, _ = strconv.Atoi(strings.TrimSpace(after))
			}
		}
		body := b[sep+4 : sep+4+n]
		out = append(out, body)
		b = b[sep+4+n:]
	}
	return out
}

// mustUnmarshal decodes a frame body into v or fails the test.
func mustUnmarshal(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
}

// TestServerLifecycle drives didChange, codeAction, didClose, an unknown method, and
// shutdown, checking the reply or diagnostic each one produces.
func TestServerLifecycle(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{
		WordReplace:    map[string]string{"leverage": "use"},
		CollapseSpaces: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var in strings.Builder
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":` +
			`{"textDocument":{"uri":"file:///a","text":"clean text"}}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didChange","params":` +
			`{"textDocument":{"uri":"file:///a"},"contentChanges":[{"text":"we leverage it"}]}}`,
		`{"jsonrpc":"2.0","id":2,"method":"textDocument/codeAction","params":` +
			`{"textDocument":{"uri":"file:///a"}}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didClose","params":` +
			`{"textDocument":{"uri":"file:///a"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"unknown/method","params":{}}`,
		`{"jsonrpc":"2.0","id":4,"method":"shutdown"}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	} {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}

	var out bytes.Buffer
	if err := NewServer(san, strings.NewReader(in.String()), &out).Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	frames := splitFrames(t, out.Bytes())
	// init, open diag, change diag, code action reply, close diag, error reply, shutdown reply.
	if len(frames) != 7 {
		t.Fatalf("frames = %d, want 7", len(frames))
	}

	// Test 0: didChange republishes diagnostics for the new text.
	var diag struct {
		Params publishDiagnosticsParams `json:"params"`
	}
	mustUnmarshal(t, frames[2], &diag)
	if len(diag.Params.Diagnostics) != 1 {
		t.Errorf("didChange diagnostics = %d, want 1", len(diag.Params.Diagnostics))
	}

	// Test 1: codeAction offers one whole-document chop.
	var action struct {
		Result []codeAction `json:"result"`
	}
	mustUnmarshal(t, frames[3], &action)
	if len(action.Result) != 1 || action.Result[0].Kind != "quickfix" {
		t.Fatalf("code actions = %+v, want one quickfix", action.Result)
	}
	if edits := action.Result[0].Edit.Changes["file:///a"]; len(edits) != 1 || edits[0].NewText != "we use it" {
		t.Errorf("action edits = %+v, want one edit to \"we use it\"", edits)
	}

	// Test 2: didClose clears the document's diagnostics.
	mustUnmarshal(t, frames[4], &diag)
	if len(diag.Params.Diagnostics) != 0 {
		t.Errorf("didClose diagnostics = %d, want 0", len(diag.Params.Diagnostics))
	}

	// Test 3: an unknown request gets a method-not-found error.
	var errReply struct {
		Error *responseError `json:"error"`
	}
	mustUnmarshal(t, frames[5], &errReply)
	if errReply.Error == nil || errReply.Error.Code != methodNotFound {
		t.Errorf("unknown method reply = %+v, want code %d", errReply.Error, methodNotFound)
	}

	// Test 4: shutdown replies with a null result.
	var shut struct {
		Result json.RawMessage `json:"result"`
	}
	mustUnmarshal(t, frames[6], &shut)
	if string(shut.Result) != "null" {
		t.Errorf("shutdown result = %s, want null", shut.Result)
	}
}

// TestCodeActionsClean checks that a clean document offers no code action.
func TestCodeActionsClean(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{
		WordReplace:    map[string]string{"leverage": "use"},
		CollapseSpaces: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var in strings.Builder
	for _, body := range []string{
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":` +
			`{"textDocument":{"uri":"file:///a","text":"clean text"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"textDocument/codeAction","params":` +
			`{"textDocument":{"uri":"file:///a"}}}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	} {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	var out bytes.Buffer
	if err := NewServer(san, strings.NewReader(in.String()), &out).Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	frames := splitFrames(t, out.Bytes())
	var action struct {
		Result []codeAction `json:"result"`
	}
	mustUnmarshal(t, frames[1], &action)
	if len(action.Result) != 0 {
		t.Errorf("code actions on clean text = %+v, want none", action.Result)
	}
}

// proseGolden is the server's full output for the prose script in TestProseGolden, captured
// before source documents were read as comments. A prose document must keep it byte for byte.
const proseGolden = "" +
	"Content-Length: 167\r\n\r\n{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"capabil" +
	"ities\":{\"textDocumentSync\":1,\"codeActionProvider\":true,\"documentFormatti" +
	"ngProvider\":true},\"serverInfo\":{\"name\":\"slop-chop\"}}}" +
	"Content-Length: 1517\r\n\r\n{\"jsonrpc\":\"2.0\",\"method\":\"textDocument/pub" +
	"lishDiagnostics\",\"params\":{\"uri\":\"file:///notes.md\",\"diagnostics\":[{" +
	"\"range\":{\"start\":{\"line\":0,\"character\":3},\"end\":{\"line\":0,\"charac" +
	"ter\":11}},\"severity\":3,\"source\":\"slop-chop\",\"code\":\"replace\",\"mess" +
	"age\":\"\\\"leverage\\\" -\\u003e \\\"use\\\"\"},{\"range\":{\"start\":{\"line" +
	"\":0,\"character\":15},\"end\":{\"line\":0,\"character\":16}},\"severity\":3," +
	"\"source\":\"slop-chop\",\"code\":\"char:—\",\"message\":\"\\\"—\\\" -\\u003e " +
	"\\\",\\\"\"},{\"range\":{\"start\":{\"line\":0,\"character\":17},\"end\":{\"li" +
	"ne\":0,\"character\":28}},\"severity\":3,\"source\":\"slop-chop\",\"code\":\"p" +
	"hrase:in order to\",\"message\":\"\\\"in order to\\\" -\\u003e \\\"to\\\"\"},{" +
	"\"range\":{\"start\":{\"line\":0,\"character\":33},\"end\":{\"line\":0,\"chara" +
	"cter\":35}},\"severity\":3,\"source\":\"slop-chop\",\"code\":\"double-space\"," +
	"\"message\":\"\\\"  \\\" -\\u003e \\\" \\\"\"},{\"range\":{\"start\":{\"line\"" +
	":0,\"character\":41},\"end\":{\"line\":0,\"character\":49}},\"severity\":3,\"s" +
	"ource\":\"slop-chop\",\"code\":\"word:powerful\",\"message\":\"\\\"powerful\\" +
	"\": word:powerful\"},{\"range\":{\"start\":{\"line\":0,\"character\":49},\"end" +
	"\":{\"line\":0,\"character\":52}},\"severity\":3,\"source\":\"slop-chop\",\"co" +
	"de\":\"semicolon\",\"message\":\"\\\"; n\\\" -\\u003e \\\". N\\\"\"},{\"range" +
	"\":{\"start\":{\"line\":0,\"character\":51},\"end\":{\"line\":0,\"character\":" +
	"74}},\"severity\":3,\"source\":\"slop-chop\",\"code\":\"structural:neg-paralle" +
	"l\",\"message\":\"\\\"not just fast, but also\\\": structural:neg-parallel\"}," +
	"{\"range\":{\"start\":{\"line\":1,\"character\":11},\"end\":{\"line\":1,\"char" +
	"acter\":19}},\"severity\":3,\"source\":\"slop-chop\",\"code\":\"replace\",\"me" +
	"ssage\":\"\\\"leverage\\\" -\\u003e \\\"use\\\"\"}]}}" +
	"Content-Length: 301\r\n\r\n{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":[{\"title" +
	"\":\"Chop the slop\",\"kind\":\"quickfix\",\"edit\":{\"changes\":{\"file:///no" +
	"tes.md\":[{\"range\":{\"start\":{\"line\":0,\"character\":0},\"end\":{\"line\"" +
	":1,\"character\":25}},\"newText\":\"We use it, to win. It is powerful. Not jus" +
	"t fast, but also cheap.\\nCafé 😀 we use more.\"}]}}}]}" +
	"Content-Length: 215\r\n\r\n{\"jsonrpc\":\"2.0\",\"id\":3,\"result\":[{\"range" +
	"\":{\"start\":{\"line\":0,\"character\":0},\"end\":{\"line\":1,\"character\":2" +
	"5}},\"newText\":\"We use it, to win. It is powerful. Not just fast, but also c" +
	"heap.\\nCafé 😀 we use more.\"}]}" +
	"Content-Length: 113\r\n\r\n{\"jsonrpc\":\"2.0\",\"method\":\"textDocument/publ" +
	"ishDiagnostics\",\"params\":{\"uri\":\"file:///notes.md\",\"diagnostics\":[]}}"

// TestProseGolden checks that a prose document gets exactly the diagnostics, chop action, and
// formatting edit it always has, so reading source files as comments leaves prose alone.
func TestProseGolden(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{
		WordReplace:     map[string]string{"leverage": "use"},
		PhraseReplace:   map[string]string{"in order to": "to"},
		CharReplace:     map[string]string{"—": ","},
		BlockWords:      []string{"powerful"},
		FlagPatterns:    map[string]string{"neg-parallel": `not just .+ but also`},
		CollapseSpaces:  true,
		SplitSemicolons: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const text = `We leverage it — in order to win.  It is powerful; not just fast, but also cheap.\n` +
		`Café 😀 we leverage more.`
	out := runServer(t, san,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":`+
			`{"textDocument":{"uri":"file:///notes.md","text":"`+text+`"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"textDocument/codeAction","params":`+
			`{"textDocument":{"uri":"file:///notes.md"},"range":{"start":{"line":0,"character":3},`+
			`"end":{"line":0,"character":3}}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"textDocument/formatting","params":`+
			`{"textDocument":{"uri":"file:///notes.md"}}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didClose","params":`+
			`{"textDocument":{"uri":"file:///notes.md"}}}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	)
	if diff := cmp.Diff(proseGolden, string(out)); diff != "" {
		t.Errorf("prose output moved (-want +got):\n%s", diff)
	}
}

// runServer frames each body as a client message, runs a server over them, and returns
// everything the server wrote.
func runServer(t *testing.T, san *sanitize.Sanitizer, bodies ...string) []byte {
	t.Helper()
	var in strings.Builder
	for _, body := range bodies {
		fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	var out bytes.Buffer
	if err := NewServer(san, strings.NewReader(in.String()), &out).Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.Bytes()
}

// TestSourceDocument checks that a source file is read as its comments: diagnostics land on
// comment text only, at UTF-16 columns of the original, cleanups are dropped, formatting edits
// nothing, and each fixable finding in range gets its own single-edit quick fix.
func TestSourceDocument(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{
		WordReplace:    map[string]string{"leverage": "use", "really": ""},
		PhraseReplace:  map[string]string{"in order to": "to"},
		BlockWords:     []string{"powerful"},
		CollapseSpaces: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const text = `package main\n\n// We leverage the  cache, a powerful one.\n` +
		`var s = \"😀 powerful\" // also powerful\n// It works really, in order\n// to win.\n`
	askFixes := func(id int, start, end string) string {
		return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"textDocument/codeAction",` +
			`"params":{"textDocument":{"uri":"file:///src/main.go"},"range":{"start":` + start +
			`,"end":` + end + `}}}`
	}
	frames := splitFrames(t, runServer(t, san,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":`+
			`{"textDocument":{"uri":"file:///src/main.go","text":"`+text+`"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"textDocument/formatting","params":`+
			`{"textDocument":{"uri":"file:///src/main.go"}}}`,
		askFixes(2, `{"line":2,"character":7}`, `{"line":2,"character":7}`),
		askFixes(3, `{"line":0,"character":0}`, `{"line":6,"character":0}`),
		askFixes(4, `{"line":3,"character":31}`, `{"line":3,"character":31}`),
		`{"jsonrpc":"2.0","method":"exit"}`,
	))
	if len(frames) != 5 {
		t.Fatalf("frames = %d, want 5", len(frames))
	}

	// Test 0: only comment text reports, never the string literal, and the emoji in the code
	// counts as two UTF-16 units in front of the second comment's word.
	type spot struct {
		Code  string
		Range Range
	}
	var diag struct {
		Params publishDiagnosticsParams `json:"params"`
	}
	mustUnmarshal(t, frames[0], &diag)
	got := make([]spot, 0, len(diag.Params.Diagnostics))
	for _, d := range diag.Params.Diagnostics {
		got = append(got, spot{Code: d.Code, Range: d.Range})
	}
	at := func(line, start, end int) Range {
		return Range{Start: Position{Line: line, Character: start}, End: Position{Line: line, Character: end}}
	}
	want := []spot{
		{Code: "replace", Range: at(2, 6, 14)},
		{Code: "word:powerful", Range: at(2, 29, 37)},
		{Code: "word:powerful", Range: at(3, 30, 38)},
		{Code: "drop:really", Range: at(4, 12, 18)},
		{Code: "phrase:in order to", Range: Range{
			Start: Position{Line: 4, Character: 20}, End: Position{Line: 5, Character: 5},
		}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diagnostics mismatch (-want +got):\n%s", diff)
	}

	// Test 1: formatting a source file returns an empty edit list.
	if !bytes.Contains(frames[1], []byte(`"result":[]`)) {
		t.Errorf("formatting = %s, want an empty result", frames[1])
	}

	// Test 2: a cursor on the replaceable word offers one quick fix with one edit.
	var action struct {
		Result []codeAction `json:"result"`
	}
	mustUnmarshal(t, frames[2], &action)
	wantFix := []codeAction{{
		Title: `slop-chop: change to "use"`,
		Kind:  "quickfix",
		Diagnostics: []Diagnostic{{
			Range: at(2, 6, 14), Severity: severityInformation, Source: "slop-chop",
			Code: "replace", Message: `"leverage" -> "use"`,
		}},
		IsPreferred: true,
		Edit: workspaceEdit{Changes: map[string][]TextEdit{
			"file:///src/main.go": {{Range: at(2, 6, 14), NewText: "use"}},
		}},
	}}
	if diff := cmp.Diff(wantFix, action.Result); diff != "" {
		t.Errorf("quick fix mismatch (-want +got):\n%s", diff)
	}

	// Test 3: the whole file offers a fix per fixable finding and none for the flags or for
	// the phrase the mask joined across a comment marker, whose edit would delete the "//".
	mustUnmarshal(t, frames[3], &action)
	titles := make([]string, 0, len(action.Result))
	for _, a := range action.Result {
		titles = append(titles, a.Title)
	}
	if diff := cmp.Diff([]string{`slop-chop: change to "use"`, "slop-chop: drop it"}, titles); diff != "" {
		t.Errorf("whole-file fixes mismatch (-want +got):\n%s", diff)
	}

	// Test 4: a cursor on a flag with no replacement offers nothing.
	mustUnmarshal(t, frames[4], &action)
	if len(action.Result) != 0 {
		t.Errorf("fixes on a flag = %+v, want none", action.Result)
	}
}

// TestSourceNoComments checks that a source type with no comment syntax gets empty
// diagnostics, no code actions, and no formatting edits, even when it holds a tell.
func TestSourceNoComments(t *testing.T) {
	t.Parallel()
	san, err := sanitize.New(sanitize.Profile{WordReplace: map[string]string{"leverage": "use"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	frames := splitFrames(t, runServer(t, san,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":`+
			`{"textDocument":{"uri":"file:///data.json","text":"{\"k\": \"we leverage it\"}"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"textDocument/codeAction","params":`+
			`{"textDocument":{"uri":"file:///data.json"},"range":{"start":{"line":0,"character":0},`+
			`"end":{"line":0,"character":20}}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":`+
			`{"textDocument":{"uri":"file:///data.json"}}}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	))
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3", len(frames))
	}
	for i, want := range []string{`"diagnostics":[]`, `"result":[]`, `"result":[]`} {
		if !bytes.Contains(frames[i], []byte(want)) {
			t.Errorf("frame %d = %s, want %s", i, frames[i], want)
		}
	}
}

// TestURIPath checks that file URIs decode to their path and other schemes pass through.
func TestURIPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In       string
		WantPath string
	}{{ // Test 0: A file URI yields its path.
		In: "file:///src/main.go", WantPath: "/src/main.go",
	}, { // Test 1: Percent escapes decode.
		In: "file:///c%3A/My%20Code/a.py", WantPath: "/c:/My Code/a.py",
	}, { // Test 2: An untitled buffer is used as it is.
		In: "untitled:Untitled-1", WantPath: "untitled:Untitled-1",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantPath, uriPath(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
