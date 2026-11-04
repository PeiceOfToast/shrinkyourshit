// SPDX-License-Identifier: AGPL-3.0-only

package flow

import (
	"fmt"
	"strings"
)

// The raw_args step (task 58, spec 5.3): the one place a user hands the
// engine ffmpeg arguments the validator does not model. The string is split
// into words by a hand written tokeniser, never handed to a shell, and
// checked against a denylist. The rules and the reason for each entry are in
// tasks/58-notes.md.
const (
	RawArgsStageInput  = "input"  // options placed before the input file
	RawArgsStageOutput = "output" // options placed after the input, before the output

	RawArgsMaxBytes = 2000
	RawArgsNoteMax  = 200
)

// RawArg is one raw_args step as resolved for a file: the stage it applies
// to, its words (already split, never shell-evaluated), and the original
// string and note for the audit line.
type RawArg struct {
	StepID string   `json:"step_id"`
	Stage  string   `json:"stage"`
	Args   []string `json:"args"`
	Raw    string   `json:"raw"`
	Note   string   `json:"note,omitempty"`
}

// rawArgsParams is the raw_args step's parameters.
type rawArgsParams struct {
	Stage string `json:"stage"`
	Args  string `json:"args"`
	Note  string `json:"note,omitempty"`
}

// shellMetacharacters are refused anywhere in the string, quoted or not.
// Nothing runs a shell, but the rule is kept as specified so that a reader
// can audit the string at a glance.
var shellMetacharacters = []string{";", "|", "&", "`", "$("}

// deviceFormats are ffmpeg input formats that read from a device or a
// virtual source instead of a file. A raw argument may not select one.
var deviceFormats = map[string]bool{
	"alsa": true, "avfoundation": true, "bktr": true, "decklink": true,
	"dshow": true, "dv1394": true, "fbdev": true, "gdigrab": true,
	"iec61883": true, "jack": true, "kmsgrab": true, "lavfi": true,
	"libcdio": true, "libdc1394": true, "openal": true, "oss": true,
	"pulse": true, "sndio": true, "v4l2": true, "video4linux2": true,
	"vfwcap": true, "x11grab": true,
}

// booleanOptions take no value, so a word after one is not its value. A
// bare word after them is an output file name or a stray positional.
var booleanOptions = map[string]bool{
	"-an": true, "-vn": true, "-sn": true, "-dn": true,
	"-stats": true, "-nostats": true, "-hide_banner": true, "-nostdin": true,
	"-shortest": true, "-copyts": true, "-re": true, "-accurate_seek": true,
	"-noaccurate_seek": true, "-autorotate": true, "-noautorotate": true,
	"-xerror": true, "-ignore_unknown": true, "-benchmark": true,
}

// ParseRawArgs checks a raw_args string and splits it into words. Every
// rejection is one error whose message names the rule, so the caller can
// attach it to the step's args field.
func ParseRawArgs(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("the arguments are empty")
	}
	if len(s) > RawArgsMaxBytes {
		return nil, fmt.Errorf("the arguments are longer than %d bytes", RawArgsMaxBytes)
	}
	if strings.ContainsAny(s, "\n\r\x00") {
		return nil, fmt.Errorf("newlines are not allowed: put the arguments on one line")
	}
	for _, m := range shellMetacharacters {
		if strings.Contains(s, m) {
			return nil, fmt.Errorf("the shell character %q is not allowed: the arguments are never run by a shell and this character is refused", m)
		}
	}
	tokens, err := splitRawArgs(s)
	if err != nil {
		return nil, err
	}
	if err := checkRawTokens(tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// checkRawTokens applies the token level denylist: input and output path
// selectors, overwrite flags, devices and bare positionals.
func checkRawTokens(tokens []string) error {
	for i, t := range tokens {
		switch {
		case t == "-i":
			return fmt.Errorf("-i sets the input file: the input path belongs to the engine")
		case t == "-y" || t == "-n":
			return fmt.Errorf("%s changes how the output file is overwritten: the output path belongs to the engine", t)
		case t == "-f" && i+1 < len(tokens) && isDevice(tokens[i+1]):
			return fmt.Errorf("-f %s reads from a device or a virtual source: raw arguments may only read files", tokens[i+1])
		}
		if !strings.HasPrefix(t, "-") && (i == 0 || !expectsValue(tokens[i-1])) {
			return fmt.Errorf("%q is a bare word: a file name or a value with no option before it; raw arguments may only set options", t)
		}
	}
	return nil
}

// isDevice reports whether an -f value names a device format or a device
// path.
func isDevice(v string) bool {
	return deviceFormats[v] || strings.HasPrefix(v, "/dev/")
}

// expectsValue reports whether the word before a bare word is an option that
// takes a value.
func expectsValue(prev string) bool {
	return strings.HasPrefix(prev, "-") && !booleanOptions[prev]
}

// splitRawArgs splits a string into words with POSIX shell quoting rules,
// without a shell: whitespace separates words, single quotes are literal,
// double quotes keep everything but \" \\ \$ and \` escapes, and a backslash
// outside quotes escapes the next character. An unbalanced quote or a
// trailing backslash is an error.
func splitRawArgs(s string) ([]string, error) {
	var (
		out   []string
		cur   strings.Builder
		inTok bool
		runes = []rune(s)
	)
	flush := func() {
		if inTok {
			out = append(out, cur.String())
			cur.Reset()
			inTok = false
		}
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case ' ', '\t':
			flush()
		case '\'':
			inTok = true
			j := i + 1
			for j < len(runes) && runes[j] != '\'' {
				cur.WriteRune(runes[j])
				j++
			}
			if j >= len(runes) {
				return nil, fmt.Errorf("unbalanced single quote: a ' has no closing '")
			}
			i = j
		case '"':
			inTok = true
			j := i + 1
			closed := false
			for j < len(runes) {
				c := runes[j]
				if c == '"' {
					closed = true
					break
				}
				if c == '\\' && j+1 < len(runes) && strings.ContainsRune("\"\\$`", runes[j+1]) {
					cur.WriteRune(runes[j+1])
					j += 2
					continue
				}
				cur.WriteRune(c)
				j++
			}
			if !closed {
				return nil, fmt.Errorf("unbalanced double quote: a \" has no closing \"")
			}
			i = j
		case '\\':
			if i+1 >= len(runes) {
				return nil, fmt.Errorf("a backslash at the end has nothing to escape")
			}
			inTok = true
			cur.WriteRune(runes[i+1])
			i++
		default:
			inTok = true
			cur.WriteRune(r)
		}
	}
	flush()
	return out, nil
}

// validateRawArgsParams checks one raw_args step's parameters. The error path
// is relative to the node.
func validateRawArgsParams(raw []byte) *paramError {
	var p rawArgsParams
	if err := decodeParams(raw, &p); err != nil {
		return perr("params", "%s", err)
	}
	switch p.Stage {
	case RawArgsStageInput, RawArgsStageOutput:
	default:
		return perr("params.stage", "want input or output, got %q", p.Stage)
	}
	if len(p.Note) > RawArgsNoteMax {
		return perr("params.note", "want at most %d characters, got %d", RawArgsNoteMax, len(p.Note))
	}
	if _, err := ParseRawArgs(p.Args); err != nil {
		return perr("params.args", "%s", err)
	}
	return nil
}

// RawArgsRefusals lists every raw_args step of a graph as a field error. The
// caller uses it only when the app setting allow_raw_args is off: the graph
// is then refused rather than having the step dropped (spec 5.3).
func RawArgsRefusals(g Graph) []FieldError {
	var errs []FieldError
	for i, n := range g.Nodes {
		if n.Type != StepRawArgs {
			continue
		}
		errs = append(errs, FieldError{
			Path:    fmt.Sprintf("nodes[%d].type", i),
			Message: "raw ffmpeg arguments are switched off in the app settings (allow_raw_args): remove this step or turn the setting on",
		})
	}
	return errs
}

// describeRawArgs is the audit line for a raw_args step: unsafe, with the
// string verbatim. Describe calls it, so the UI reads the same text.
func describeRawArgs(p rawArgsParams, guard string) string {
	where := "as input options, before the input file"
	if p.Stage == RawArgsStageOutput {
		where = "as output options, after the input file"
	}
	line := fmt.Sprintf("%sUNSAFE raw ffmpeg arguments %s: %s", guard, where, p.Args)
	if p.Note != "" {
		line += " Note: " + p.Note
	}
	return line
}
