package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/fatih/color"
)

// mergeReferenceConfig merges the monorepo's opencode/reference config (JSONC)
// into the global OpenCode config at dstPath. It is idempotent (running twice
// changes nothing) and never overwrites third-party keys or entries:
//
//   - Keys absent from the destination are added ($schema, skills, instructions,
//     mcp.servers.ai-router and any other key of the reference).
//   - The skills/instructions arrays are rebuilt so dctx-managed entries are
//     deduped and keep their planned order (kiro/skills before opencode/skills
//     — in an ID collision the later source wins); third-party entries are kept
//     untouched at the front of the array.
//   - An existing mcp.servers.ai-router entry is NEVER overwritten (the user
//     may have registered it with a different venv path or via
//     'opencode mcp add'); other MCP servers are untouched.
//   - Monorepo paths in skills/instructions are rebased from ~ to the real
//     clone path (cfg repo path) and written expanded, per the port plan
//     ("dctx resolve o caminho real e grava expandido").
//
// The result is written as plain JSON into whatever destination file exists
// (opencode.json or opencode.jsonc; a fresh machine gets opencode.json).
// Returns true when the destination file changed.
func mergeReferenceConfig(repoPath, dstPath string) (bool, error) {
	before, err := os.ReadFile(dstPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading destination: %w", err)
	}

	merged, changed, err := mergeInto(before, repoPath)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encoding merged config: %w", err)
	}
	out = append(out, '\n')
	if jsonEqual(before, out) {
		return false, nil
	}
	return true, os.WriteFile(dstPath, out, 0o644)
}

// mergeInto merges the reference config into the raw destination bytes and
// returns the merged map plus whether anything changed.
func mergeInto(dstRaw []byte, repoPath string) (map[string]interface{}, bool, error) {
	ref, err := readReference(repoPath)
	if err != nil {
		return nil, false, err
	}

	dstMap := make(map[string]interface{})
	if len(dstRaw) > 0 {
		var dstRawMap map[string]interface{}
		if err := json.Unmarshal(stripJSONC(dstRaw), &dstRawMap); err != nil {
			return nil, false, fmt.Errorf("parsing destination config: %w", err)
		}
		for k, v := range dstRawMap {
			dstMap[k] = v // keep every existing key — third-party data is sacred
		}
	}

	changed := false
	// Keys of the reference absent in the destination are added.
	for k, v := range ref {
		switch k {
		case "skills", "instructions":
			merged, keyChanged, ok := mergeManagedStrings(dstMap[k], v, repoPath)
			if ok {
				if keyChanged {
					dstMap[k] = merged
					changed = true
				}
			} else {
				// Destination has an unexpected shape (e.g. string instead of
				// array) — leave it alone rather than replacing user data.
				color.Yellow("  ⚠  %s no config global tem formato inesperado; entrada reuso ignorada", k)
			}
		case "mcp":
			if keyChanged, ok := mergeMCP(dstMap, v); ok {
				changed = changed || keyChanged
			}
		default:
			if _, exists := dstMap[k]; !exists {
				dstMap[k] = v
				changed = true
			}
		}
	}
	return dstMap, changed, nil
}

// readReference loads and parses the reference config (JSONC) from the repo.
func readReference(repoPath string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(filepath.Join(repoPath, "opencode", "config", "opencode.reference.jsonc"))
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil // older monorepo without opencode/: nothing to manage
	}
	if err != nil {
		return nil, fmt.Errorf("reading reference config: %w", err)
	}
	var ref map[string]interface{}
	if err := json.Unmarshal(stripJSONC(raw), &ref); err != nil {
		return nil, fmt.Errorf("parsing reference config: %w", err)
	}
	return ref, nil
}

// mergeManagedStrings rebuilds a string array (skills or instructions):
// third-party entries whose value is not a dctx-managed path stay first (order
// preserved), then managed entries are appended deduped in reference order.
// Managed entries retain their reference order so ID precedence holds.
// Returns (newArray, changed, ok); ok=false when dst has an unusable shape.
func mergeManagedStrings(dstVal, refVal interface{}, repoPath string) (interface{}, bool, bool) {
	refArr, ok := refVal.([]interface{})
	if !ok {
		return nil, false, false
	}
	managed := make([]string, 0, len(refArr))
	for _, e := range refArr {
		s, ok := e.(string) // non-string reference entries are skipped defensively
		if !ok {
			continue
		}
		managed = append(managed, rebaseManagedPath(s, repoPath))
	}
	if len(managed) == 0 {
		return dstVal, false, true
	}

	var kept []interface{}
	existing, _ := dstVal.([]interface{})
	for _, e := range existing {
		s, isStr := e.(string)
		// dctx-managed path (also when written in ~/… or {env:HOME}/… form by a
		// previous manual setup) — rewritten in canonical form below.
		if isStr && containsPath(managed, s, repoPath) {
			continue
		}
		kept = append(kept, e)
	}
	newArr := make([]interface{}, 0, len(kept)+len(managed))
	newArr = append(newArr, kept...)
	for _, m := range managed {
		newArr = append(newArr, m)
	}
	// Rewrite only when the desired array differs from the current one: the
	// de-dup on entry removal above is semantic, not a verdict of change.
	if arraysSemanticallyEqual(existing, newArr) {
		return dstVal, false, true
	}
	return newArr, true, true
}

// arraysSemanticallyEqual compares decoded JSON arrays, treating string entries
// as equal when their normalized path forms match (~/ and {env:HOME}/ forms,
// canonical vs expanded monorepo paths, formatting), so an already-satisfied
// destination is left byte-for-byte untouched.
func arraysSemanticallyEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		as, aStr := a[i].(string)
		bs, bStr := b[i].(string)
		if aStr && bStr {
			if normalizeEntry(as) != normalizeEntry(bs) {
				return false
			}
			continue
		}
		if aStr != bStr {
			return false
		}
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// mergeMCP merges the reference "mcp" object into the destination map without
// overwriting server entries that already exist (in particular ai-router).
// The destination "mcp" key is only created when there is something to add.
// Returns (changed, processed); unprocessable shapes yield (false, processed=true).
func mergeMCP(dstMap map[string]interface{}, refVal interface{}) (bool, bool) {
	refObj, ok := refVal.(map[string]interface{})
	if !ok {
		return false, true
	}
	refServers, ok := refObj["servers"].(map[string]interface{})
	if !ok || len(refServers) == 0 {
		return false, true
	}

	dstObj, _ := dstMap["mcp"].(map[string]interface{})
	if dstObj == nil {
		if _, exists := dstMap["mcp"]; exists {
			return false, true // destination has a non-object mcp — leave it alone
		}
		dstObj = make(map[string]interface{})
	}
	dstServers, _ := dstObj["servers"].(map[string]interface{})
	if dstServers == nil {
		if _, exists := dstObj["servers"]; exists {
			return false, true
		}
		dstServers = make(map[string]interface{})
		dstObj["servers"] = dstServers
	}

	changed := false
	for name, server := range refServers {
		if _, exists := dstServers[name]; exists {
			continue // never overwrite an existing server entry (user-owned)
		}
		dstServers[name] = server
		changed = true
	}
	if !changed {
		return false, true
	}
	// Only now attach the (possibly new) "mcp" object, so a no-change merge
	// never introduces empty "mcp"/"servers" keys into the destination.
	dstMap["mcp"] = dstObj
	return changed, true
}

// mergeSatisfied reports whether the destination config already contains every
// managed entry (schema, skills/instructions in reference order, ai-router
// server). It tolerates extra third-party keys, extra array entries, {env:HOME}
// style paths and different formatting — the merge itself never rewrites them.
func mergeSatisfied(repoPath, dstPath string) (bool, error) {
	dstRaw, err := os.ReadFile(dstPath)
	if err != nil {
		return false, err
	}
	_, changed, err := mergeInto(dstRaw, repoPath)
	if err != nil {
		return false, err
	}
	// The satisfaction check is: merging again changes nothing.
	return !changed, nil
}

// rebaseManagedPath expands a reference path entry. "~/" is expanded to the
// user's home; "~/" followed by the canonical monorepo location is rebased to
// the actual clone path (repoPath/env config), so machines that cloned the
// monorepo elsewhere still get working entries. Returns an absolute expanded path.
func rebaseManagedPath(entry, repoPath string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(entry, "~") {
		entry = filepath.Join(home, entry[1:])
	}
	entry = filepath.Clean(entry)
	// Rebase the canonical ~/git/dev-context prefix used in the reference onto
	// the real clone path from the dctx config.
	if home != "" {
		if defaultRoot := filepath.Join(home, "git", "dev-context"); defaultRoot != repoPath {
			if rel, err := filepath.Rel(defaultRoot, entry); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.Join(repoPath, rel)
			}
		}
	}
	return entry
}

// containsPath reports whether entry is one of the managed paths modulo home
// expansion and monorepo rebase. Entries written in ~/, {env:HOME}/… or
// ~/git/dev-context/… form (e.g. by the manual setup documented in the
// monorepo README) still count as us — they are dctx's own paths, not
// third-party data.
func containsPath(paths []string, entry, repoPath string) bool {
	if p := normalizeEntry(entry); containsNormalized(paths, p) {
		return true
	}
	return containsNormalized(paths, rebaseManagedPath(entry, repoPath))
}

// containsNormalized compares an already-normalized entry against paths that
// are normalized on the fly (paths come pre-expanded from rebaseManagedPath,
// so a straight comparison is enough, but normalization keeps this safe for
// future entry shapes).
func containsNormalized(paths []string, entry string) bool {
	for _, p := range paths {
		if normalizeEntry(p) == entry {
			return true
		}
	}
	return false
}

// normalizeEntry canonicalizes a config path entry for comparison: expands ~
// (and {env:HOME}) to the real home and cleans the path.
func normalizeEntry(entry string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(entry, "{env:HOME}") {
		entry = filepath.Join(home, entry[len("{env:HOME}"):])
	} else if strings.HasPrefix(entry, "~") {
		entry = filepath.Join(home, entry[1:])
	}
	// Also normalize path-embedded {env:HOME} occurrences defensively (future entries).
	if home != "" {
		entry = strings.ReplaceAll(entry, "{env:HOME}", home)
	}
	return filepath.Clean(entry)
}

// jsonEqual compares two marshaled JSON snippets by canonical bytes.
func jsonEqual(a, b []byte) bool {
	return strings.TrimSpace(string(a)) == strings.TrimSpace(string(b))
}

// ── JSONC support ─────────────────────────────────────────────────────────────

// stripJSONC removes //, block /**/ comments and trailing commas from JSONC
// content so encoding/json can parse it. String contents ("/" in URLs, braces
// in values) are left untouched.
func stripJSONC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	i := 0
	for i < len(data) {
		c := data[i]
		if inString {
			out = append(out, c)
			if c == '\\' && i+1 < len(data) {
				out = append(out, data[i+1])
				i += 2
				continue
			}
			if c == '"' {
				inString = false
			}
			i++
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
			i++
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			i += 2
		default:
			out = append(out, c)
			i++
		}
	}
	// Remove trailing commas: a comma followed only by whitespace and ] or }.
	return stripTrailingCommas(out)
}

// stripTrailingCommas drops commas that are directly followed (ignoring
// whitespace/newlines) by a closing bracket, a JSONC tolerance that the
// reference config uses.
func stripTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			out = append(out, c)
			if c == '\\' && i+1 < len(data) {
				out = append(out, data[i+1])
				i++
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == ']' || data[j] == '}') {
				continue // drop the trailing comma
			}
		}
		out = append(out, c)
	}
	return out
}
