//go:build js && wasm

// Command wasm compiles the slop-chop rules engine for the browser. It registers a
// small set of functions on the JavaScript global object so slop-chop.com can chop
// text client side, with no server and no model.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall/js"

	"github.com/dcadolph/slop-chop/internal/jsonutil"
	"github.com/dcadolph/slop-chop/internal/source"
	"github.com/dcadolph/slop-chop/rewrite/prompt"
	"github.com/dcadolph/slop-chop/sanitize"
)

// version is stamped by the Makefile at build time.
var version = "dev"

// ErrRequest marks a malformed call from the page, like a missing argument or JSON
// that does not decode.
var ErrRequest = errors.New("bad request")

// chopRequest is the payload slopChop accepts, decoded from its single JSON argument.
type chopRequest struct {
	// Text is the input to clean.
	Text string `json:"text"`
	// Profile is the full profile to apply. The page builds it, defaults included, so
	// the engine sees exactly what the settings panel shows.
	Profile sanitize.Profile `json:"profile"`
	// Presets names built-in presets merged on top, with the profile winning on any
	// conflict, matching the CLI's --preset flag.
	Presets []string `json:"presets"`
	// Voice is the personal layer, applied over the presets the way the CLI applies a voice
	// file, so every caller folds it with the engine's one mapping rather than its own copy.
	Voice sanitize.Voice `json:"voice"`
}

// profile resolves the request into the profile to run: the given profile, the presets
// merged under it, and then the voice on top.
func (r chopRequest) profile() (sanitize.Profile, error) {
	p := r.Profile
	if len(r.Presets) > 0 {
		merged, err := sanitize.ApplyPresets(p, r.Presets...)
		if err != nil {
			return sanitize.Profile{}, err
		}
		p = merged
	}
	return p.WithVoice(r.Voice), nil
}

// chopResult is what slopChop returns, encoded as JSON.
type chopResult struct {
	// Output is the cleaned text.
	Output string `json:"output"`
	// Findings lists every rule match against the original text.
	Findings []sanitize.Finding `json:"findings"`
	// Score rates the original text from 0 to 100 by how densely it carries the profile's
	// patterns. It is a lint result rather than a reading on who wrote the text.
	Score sanitize.Score `json:"score"`
	// ScoreAfter rates the cleaned output on the same scale, so the page can show how far
	// the chop moved the needle.
	ScoreAfter sanitize.Score `json:"scoreAfter"`
	// Remaining is what a check reports on the cleaned output: the tells no rule can
	// rewrite, left for the author. A finding with a replacement was fixed, so the page
	// counts those from Findings and these from here.
	Remaining []sanitize.Finding `json:"remaining"`
	// Band is the published range the original score falls in, so a caller that colors or
	// labels the score uses the engine's boundaries rather than its own copy of them.
	Band sanitize.Band `json:"band"`
	// BandAfter is the range the cleaned output's score falls in.
	BandAfter sanitize.Band `json:"bandAfter"`
}

// main registers the engine functions on the JavaScript global object and blocks
// forever, keeping the WASM instance alive for the page.
func main() {
	js.Global().Set("slopChop", js.FuncOf(chop))
	js.Global().Set("slopDefaults", js.FuncOf(defaults))
	js.Global().Set("slopPresets", js.FuncOf(presets))
	js.Global().Set("slopScore", js.FuncOf(scoreOnly))
	js.Global().Set("slopRewritePrompt", js.FuncOf(rewritePrompt))
	js.Global().Set("slopJudgePrompt", js.FuncOf(judgePrompt))
	js.Global().Set("slopVersion", js.FuncOf(engineVersion))
	js.Global().Set("slopEditor", js.FuncOf(editor))
	select {}
}

// chop runs the rules pass. It takes one JSON string argument shaped like chopRequest
// and returns a JSON string shaped like chopResult, or {"error": "..."} when the
// request or the profile does not hold up.
func chop(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return errJSON(errors.Join(ErrRequest, errors.New("slopChop takes one JSON argument")))
	}
	var req chopRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return errJSON(errors.Join(ErrRequest, err))
	}
	profile, err := req.profile()
	if err != nil {
		return errJSON(err)
	}
	s, err := sanitize.New(profile)
	if err != nil {
		return errJSON(err)
	}
	out, findings := s.Fix(req.Text)
	score, after := s.Score(req.Text), s.Score(out)
	return marshal(chopResult{
		Output:     out,
		Findings:   jsonutil.OrEmpty(findings),
		Score:      score,
		ScoreAfter: after,
		Remaining:  jsonutil.OrEmpty(s.Check(out)),
		Band:       score.Band(),
		BandAfter:  after.Band(),
	})
}

// scoreResult is what slopScore returns, encoded as JSON.
type scoreResult struct {
	// Score rates the text on the same scale slopChop reports.
	Score sanitize.Score `json:"score"`
	// Band is the published range the score falls in.
	Band sanitize.Band `json:"band"`
}

// scoreOnly scores a text without fixing it. It takes the same JSON argument as slopChop
// and skips the rewrite and the second scoring pass, which is most of the work, so a
// caller that needs only the number pays for only the number.
func scoreOnly(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return errJSON(errors.Join(ErrRequest, errors.New("slopScore takes one JSON argument")))
	}
	var req chopRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return errJSON(errors.Join(ErrRequest, err))
	}
	profile, err := req.profile()
	if err != nil {
		return errJSON(err)
	}
	s, err := sanitize.New(profile)
	if err != nil {
		return errJSON(err)
	}
	score := s.Score(req.Text)
	return marshal(scoreResult{Score: score, Band: score.Band()})
}

// editorRequest is the payload slopEditor accepts. An editor hands over the raw contents of
// the files the command line would discover, and the engine layers them with the command
// line's own precedence, so the editor holds no copy of that rule.
type editorRequest struct {
	// Text is the document to check or fix.
	Text string `json:"text"`
	// As names the source type the document is read as, like .go or Makefile, so only its
	// comments are checked. Empty reads the document as prose.
	As string `json:"as"`
	// Fix asks for the fixed prose as well as the findings. It is ignored for source, whose
	// code a whole-document fix must never touch.
	Fix bool `json:"fix"`
	// Project is the raw JSON of the nearest .slop-chop.json, or empty when there is none.
	Project string `json:"project"`
	// Voice is the raw JSON of the personal voice file, or empty when there is none.
	Voice string `json:"voice"`
	// Presets names built-in presets, as the --preset flag does.
	Presets []string `json:"presets"`
	// Dialect overrides the profile's dialect when set.
	Dialect string `json:"dialect"`
}

// editorResult is what slopEditor returns, encoded as JSON.
type editorResult struct {
	// Findings lists every rule match against the document, offsets in bytes of its UTF-8.
	Findings []sanitize.Finding `json:"findings"`
	// Output is the fixed prose when Fix was asked for, and empty otherwise.
	Output string `json:"output,omitempty"`
	// Scanned is false when the document's source type has no comment syntax, so nothing
	// was read.
	Scanned bool `json:"scanned"`
}

// editor checks or fixes one document for an editor extension. It takes one JSON string
// argument shaped like editorRequest and returns a JSON string shaped like editorResult, or
// {"error": "..."} when the request, a profile file, or a voice file does not hold up.
func editor(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return errJSON(errors.Join(ErrRequest, errors.New("slopEditor takes one JSON argument")))
	}
	var req editorRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return errJSON(errors.Join(ErrRequest, err))
	}
	layers := sanitize.Layers{Dialect: req.Dialect, Presets: req.Presets}
	if req.Project != "" {
		p, err := sanitize.Load(strings.NewReader(req.Project))
		if err != nil {
			return errJSON(fmt.Errorf(".slop-chop.json: %w", err))
		}
		layers.Project = &p
	}
	if req.Voice != "" {
		v, err := sanitize.LoadVoice(strings.NewReader(req.Voice))
		if err != nil {
			return errJSON(fmt.Errorf("voice.json: %w", err))
		}
		layers.Voice = v
	}
	profile, err := layers.Profile()
	if err != nil {
		return errJSON(err)
	}
	s, err := sanitize.New(profile)
	if err != nil {
		return errJSON(err)
	}
	if req.As != "" {
		findings, ok := source.CheckComments(s, req.As, req.Text)
		if err := sanitize.Locate(req.Text, findings); err != nil {
			return errJSON(err)
		}
		return marshal(editorResult{Findings: jsonutil.OrEmpty(findings), Scanned: ok})
	}
	res := editorResult{Scanned: true}
	if req.Fix {
		res.Output, res.Findings = s.Fix(req.Text)
	} else {
		res.Findings = s.Check(req.Text)
	}
	res.Findings = jsonutil.OrEmpty(res.Findings)
	return marshal(res)
}

// defaults returns the built-in default profile as JSON, so the page can render the
// settings panel from the same source of truth the CLI uses.
func defaults(_ js.Value, _ []js.Value) any {
	return marshal(sanitize.DefaultProfile())
}

// presets returns the built-in preset names as a JSON array.
func presets(_ js.Value, _ []js.Value) any {
	return marshal(sanitize.PresetNames())
}

// rewritePrompt returns the system prompt for the model rewrite pass as
// {"system": "..."}. It takes the profile JSON so the profile's tone notes shape the
// prompt, exactly as they do in the CLI. The page sends the prompt to whichever model
// backend the user configured.
func rewritePrompt(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return errJSON(errors.Join(ErrRequest, errors.New("slopRewritePrompt takes one JSON argument")))
	}
	var p sanitize.Profile
	if err := json.Unmarshal([]byte(args[0].String()), &p); err != nil {
		return errJSON(errors.Join(ErrRequest, err))
	}
	return marshal(map[string]string{"system": prompt.System(p.Tone, nil)})
}

// judgePrompt returns the system prompt for the meaning check as {"system": "..."},
// the same instruction the CLI's --verify pass sends.
func judgePrompt(_ js.Value, _ []js.Value) any {
	return marshal(map[string]string{"system": prompt.Judge()})
}

// engineVersion returns the stamped build version.
func engineVersion(_ js.Value, _ []js.Value) any {
	return version
}

// marshal encodes v as a JSON string for the page, falling back to an error payload
// when encoding fails.
func marshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return errJSON(err)
	}
	return string(b)
}

// errJSON wraps an error in the {"error": "..."} payload the page checks for.
func errJSON(err error) string {
	b, mErr := json.Marshal(map[string]string{"error": err.Error()})
	if mErr != nil {
		return `{"error":"encode failed"}`
	}
	return string(b)
}
