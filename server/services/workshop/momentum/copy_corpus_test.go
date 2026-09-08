package momentum

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// copyFields are the Detection fields that reach a user's screen. Anything a
// detector writes into one of these is a claim the product is making on the
// host's behalf.
var copyFields = map[string]bool{
	"Headline":            true,
	"Description":         true,
	"CtaLabel":            true,
	"KickerLabel":         true,
	"AtmosphereLine":      true,
	"EvidenceQuote":       true,
	"EvidenceAttribution": true,
}

// wantCopyCorpus is every literal string the package can put in front of a
// host. It is a golden list, not an assertion about wording: adding a line
// here should be a deliberate act a reviewer sees, because the failure mode
// this guards is copy that asserts something the detector never measured.
//
// The rule (docs/workshop.md → "Adding a new detector"): every user-visible
// field must be derivable from a value the detector read from storage.
// Counts, names and dates come from the query. Social proof needs a query
// that measured it. A citation needs a real source.
//
// #2892 is why this exists. "People keep asking about the {gear}" had no
// query behind it; two detectors carried invented quotations attributed to
// real published sources; "Most of the circle wants this" described a
// threshold measured over respondents, not the circle.
//
// Format strings appear verbatim (with their verbs) — the substituted values
// are measured, which is the point.
var wantCopyCorpus = []string{
	// active_quest_detector.go
	"Hosted %d times before",
	"On the way",
	"Schedule %s again",
	"Schedule it",
	"You've hosted it %d times — worth keeping the rhythm going.",
	// bring_back.go
	"Bring back %s",
	"Bring it back",
	"Ran %d times, then quiet",
	"The rhythm is worth keeping.",
	"Things this circle has loved",
	// calendar_gap_detector.go
	"The rhythm is real — worth a spark.",
	// esm_repeat_signal_detector.go + per_event.go
	"%d of %d said yes — pulse came back",
	"%d of %d said yes, window closed",
	"Confirm next %s",
	"Good time to get it back on the calendar.",
	"Just wrapped",
	"Just wrapped %s",
	"Make %s a weekly thing",
	"Make it weekly",
	"Most who answered want this on the calendar — propose a cadence.",
	"Pulse came back",
	"Round %d already on deck",
	"Worth doing again",
	"Worth keeping the rhythm going.",
	"You've hosted it %d times — the circle knows the drill.",
	// seasonal_trigger_detector.go
	"It ran around this time last year — worth doing again.",
	"Ran around this time last year",
	"This time last year",
}

// TestCopyCorpus_IsAccountedFor walks the package's source and collects every
// string literal assigned to a user-visible Detection field, then diffs it
// against wantCopyCorpus. New copy fails the test until it is listed, which
// is the review checkpoint — the diff is where someone asks "did we measure
// that?".
func TestCopyCorpus_IsAccountedFor(t *testing.T) {
	got := collectDetectionCopy(t)

	want := make([]string, len(wantCopyCorpus))
	copy(want, wantCopyCorpus)
	sort.Strings(want)

	wantSet := map[string]bool{}
	for _, s := range want {
		wantSet[s] = true
	}
	gotSet := map[string]bool{}
	for _, s := range got {
		gotSet[s] = true
	}

	for _, s := range got {
		if !wantSet[s] {
			t.Errorf("undeclared user-visible copy %q\n"+
				"Add it to wantCopyCorpus — and first confirm the detector "+
				"measured everything it asserts (docs/workshop.md → Adding a "+
				"new detector).", s)
		}
	}
	for _, s := range want {
		if !gotSet[s] {
			t.Errorf("wantCopyCorpus lists %q but no detector emits it; "+
				"drop the stale entry", s)
		}
	}
}

// TestCopyCorpus_CarriesNoInventedAttribution — evidence lines quote a named
// source, so an unsourced one is the highest-cost fabrication the package can
// ship. Both detectors that carried one were shipping text no source ever
// said (#2892). If evidence returns, it arrives with a real citation and this
// test is the place to record that decision.
func TestCopyCorpus_CarriesNoInventedAttribution(t *testing.T) {
	for _, s := range wantCopyCorpus {
		if strings.Contains(s, "Putnam") || strings.Contains(s, "Harvard") {
			t.Errorf("copy %q attributes a claim to a named source; "+
				"evidence must carry a real quote and a real citation", s)
		}
	}
}

// collectDetectionCopy parses every non-test .go file in this directory and
// returns the sorted, de-duplicated set of user-visible copy strings.
//
// Copy reaches a Detection field two ways, and both count: written straight
// into the composite literal, or built into a local first (the ESM paths
// compute an atmosphere line before choosing a variant). The second kind is
// resolved per function — every string a local is assigned anywhere in the
// function that eventually feeds a copy field. That over-collects in
// principle; in this package it is exact, and over-collecting is the safe
// direction for a gate whose job is to make copy visible.
func collectDetectionCopy(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	seen := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		// Function bodies are the unit: a local's assignments and the
		// composite literal that consumes it always share one.
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			for _, s := range copyInFunc(fn.Body) {
				seen[s] = true
			}
			return false
		})
	}

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// copyInFunc returns the user-visible copy literals in one function body.
func copyInFunc(body *ast.BlockStmt) []string {
	// Pass 1: every string literal each local is assigned.
	locals := map[string][]string{}
	record := func(lhs, rhs []ast.Expr) {
		for i, target := range lhs {
			ident, ok := target.(*ast.Ident)
			if !ok || i >= len(rhs) {
				continue
			}
			locals[ident.Name] = append(locals[ident.Name], stringLiterals(rhs[i])...)
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			record(s.Lhs, s.Rhs)
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(s.Names))
			for i, nm := range s.Names {
				names[i] = nm
			}
			record(names, s.Values)
		}
		return true
	})

	// Pass 2: the copy fields themselves, resolving locals through pass 1.
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || !copyFields[key.Name] {
			return true
		}
		if ident, ok := kv.Value.(*ast.Ident); ok {
			out = append(out, locals[ident.Name]...)
			return true
		}
		out = append(out, stringLiterals(kv.Value)...)
		return true
	})
	return out
}

// stringLiterals pulls the literal strings out of a copy-field value or an
// assignment's right-hand side. A bare literal is itself; a fmt.Sprintf is
// its format string (the arguments are the measured values). A call into a
// helper yields nothing here — the helper's own literals are collected when
// the walk reaches its body.
func stringLiterals(v ast.Expr) []string {
	switch e := v.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return nil
		}
		s, err := strconv.Unquote(e.Value)
		if err != nil {
			return nil
		}
		return []string{s}
	case *ast.CallExpr:
		// fmt.Sprintf("...", args) and friends: the format string is the copy.
		for _, arg := range e.Args {
			if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				return []string{s}
			}
		}
	}
	return nil
}
