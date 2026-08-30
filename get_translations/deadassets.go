package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// pluralSuffix matches the "-plural", "-plural-2", etc. suffixes loco appends to the base asset id
// when representing individual CLDR plural categories as pseudo-assets.
var pluralSuffix = regexp.MustCompile(`-plural(-\d+)?$`)

// trailingPrintfParam strips a trailing " %(name)s" / " %d" / " %s" style placeholder from an asset
// id. Backend, web, www, and android all reference the placeholder as part of the literal id, but
// Xcode's String Catalog symbol generator drops it (the value becomes a function argument instead),
// so only the iOS mangling needs the stripped form.
var trailingPrintfParam = regexp.MustCompile(`\s+%(\([\w-]+\))?[a-zA-Z]$`)

// defaultExcludes are asset ids and id prefixes (see isExcluded) that are always omitted from the
// dead-asset report, regardless of what's passed via --exclude. These are known to be unused on
// purpose rather than genuinely dead.
var defaultExcludes = []string{"special-talk.", "%d hours", "%d minutes", "role.", "itunes.app-store.", "app-store."}

// findDeadAssets fetches every asset from loco and, for each configured repo, checks whether the
// asset appears to be referenced anywhere in it. Loco's tags are not used to scope which platforms
// are checked for a given asset: in practice tags drift out of date (an asset added for one platform
// gets reused on another without re-tagging), so every asset is checked against every repo that was
// provided. The one exception is the plistTagName ("ios-plist") tag: those assets are remapped by
// updateiOSAssetsCatalog (see ioscatalog.go's plistAssetMap) into specific Apple Info.plist keys
// (e.g. NSCameraUsageDescription) rather than an id-derived symbol, so they never show up under
// their loco id in any repo and are always excluded regardless of tag drift elsewhere. excludes
// lists additional asset ids and id prefixes (see isExcluded) to omit from consideration, on top of
// defaultExcludes, for assets that are known to be unused on purpose (e.g. staged for a future
// date).
func findDeadAssets(apiKey string, repoPaths map[string]string, excludes []string) error {
	excludes = append(append([]string{}, defaultExcludes...), excludes...)

	assets, err := getAssets(apiKey, "")
	if err != nil {
		return fmt.Errorf("error fetching assets: %w", err)
	}

	indexes := make(map[string]*repoIndex, len(repoPaths))
	for platform, path := range repoPaths {
		if path == "" {
			continue
		}
		idx, err := buildRepoIndex(platform, path)
		if err != nil {
			return fmt.Errorf("error indexing %s repo at %s: %w", platform, path, err)
		}
		indexes[platform] = idx
	}
	if len(indexes) == 0 {
		return fmt.Errorf("no repo paths provided; supply at least one of --backend --web --www --android --ios")
	}

	ids := make([]string, 0, len(assets))
	seen := make(map[string]bool, len(assets))
	excluded := 0
	for _, asset := range assets {
		id := normalizeAssetID(asset.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if isExcluded(id, excludes) || hasTag(asset.Tags, plistTagName) {
			excluded++
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	hasChild := childAssetSet(ids)

	dead := make([]string, 0)
	for _, id := range ids {
		if !usedAnywhere(id, hasChild[id], indexes) {
			dead = append(dead, id)
		}
	}

	sort.Strings(dead)
	for _, id := range dead {
		fmt.Println(id)
	}
	fmt.Fprintf(os.Stderr, "\n%d of %d distinct assets appear unused across %d checked platform(s) (%d excluded)\n",
		len(dead), len(seen), len(indexes), excluded)

	return nil
}

// hasTag reports whether tags contains tag, matched case-insensitively since loco's own tag casing
// (e.g. "ios-plist" vs. "iOS-plist") isn't guaranteed consistent.
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// isExcluded reports whether id should be omitted from dead-asset consideration because it matches
// one of excludes. An exclude entry ending in "." is a prefix match (e.g. "special-talk." excludes
// "special-talk.202509", "special-talk.202609", etc.); any other entry is an exact match.
func isExcluded(id string, excludes []string) bool {
	for _, ex := range excludes {
		if strings.HasSuffix(ex, ".") {
			if strings.HasPrefix(id, ex) {
				return true
			}
		} else if id == ex {
			return true
		}
	}
	return false
}

func usedAnywhere(assetID string, hasChild bool, indexes map[string]*repoIndex) bool {
	for platform, idx := range indexes {
		if idx.hasDocListParent(assetID) {
			return true
		}
		if platform == "backend" && idx.localeIdentifiers[validConstant(assetID)] {
			return true
		}
		if platform == "www" && idx.containsBareID(assetID) {
			return true
		}
		for _, candidate := range searchCandidates(platform, assetID, hasChild) {
			if idx.contains(candidate) {
				return true
			}
		}
	}
	return false
}

// childAssetSet returns, for every id in ids, whether some other id in ids is a strict descendant of
// it under i18next's dot-namespacing (e.g. "nav.reports.averages.group" is a child of
// "nav.reports.averages"). ids must be sorted: every id sharing the "id." prefix then occupies a
// contiguous range starting at the first entry >= "id.", regardless of what non-dot continuations
// (e.g. "id-legacy", "id other") happen to sort in between "id" and that range.
func childAssetSet(ids []string) map[string]bool {
	hasChild := make(map[string]bool, len(ids))
	for _, id := range ids {
		prefix := id + "."
		i := sort.SearchStrings(ids, prefix)
		if i < len(ids) && strings.HasPrefix(ids[i], prefix) {
			hasChild[id] = true
		}
	}
	return hasChild
}

// normalizeAssetID collapses loco's plural-category pseudo ids (e.g. "%d hours-plural-2") back to
// their base id (e.g. "%d hours").
func normalizeAssetID(id string) string {
	return pluralSuffix.ReplaceAllString(id, "")
}

// repoIndex holds the concatenated contents of every relevant source file in a repo, so repeated
// substring lookups don't require re-walking the filesystem.
type repoIndex struct {
	content string
	// docListParents holds every asset id passed as the "asset" prop to hourglass-js's <DocList>
	// component. DocList calls t(asset, {returnObjects: true}) and renders every direct child value
	// it gets back, so those children are used at runtime even though their literal dotted ids never
	// appear anywhere in source.
	docListParents map[string]bool
	// localeIdentifiers holds every Go identifier referenced as "locale.<Identifier>" in the backend
	// repo. get_translations' own "assets" command generates locale/asset_ids.go, a file of Go
	// constants named by mangling each asset id through validConstant; application code then
	// references assets exclusively via that constant (e.g. locale.EmailSubmitreminderSubject_Monthyear),
	// never as the literal quoted asset id string, so the plain substring search in searchCandidates
	// can never find backend usage on its own.
	localeIdentifiers map[string]bool
}

func (r *repoIndex) contains(s string) bool {
	return strings.Contains(r.content, s)
}

// containsBareID reports whether id appears in r.content as a standalone token: preceded and
// followed by a character that isn't part of an asset-id-like token (letter, digit, underscore,
// dot, or hyphen), or by the start/end of the content. This is for www (Hugo), where page front
// matter references an asset id unquoted (e.g. "title_key: www.details.security" in
// content/en/security.html, consumed by {{ i18n .Params.title_key }}), so the quoted-literal
// candidates from searchCandidates never match. The boundary check still guards against false
// positives when one id is a dot-namespaced prefix of another (e.g. "nav.reports.monthly" inside
// "nav.reports.monthly.summary").
func (r *repoIndex) containsBareID(id string) bool {
	if id == "" {
		return false
	}
	content := r.content
	start := 0
	for {
		i := strings.Index(content[start:], id)
		if i < 0 {
			return false
		}
		idx := start + i
		before := idx == 0 || !isIDTokenChar(content[idx-1])
		after := idx+len(id) >= len(content) || !isIDTokenChar(content[idx+len(id)])
		if before && after {
			return true
		}
		start = idx + 1
	}
}

func isIDTokenChar(b byte) bool {
	return b == '.' || b == '-' || b == '_' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// docListAssetProp matches the "asset" prop of hourglass-js's <DocList> component, e.g.
// <DocList asset="docs.faq.moves.a" />.
var docListAssetProp = regexp.MustCompile(`<DocList\b[^>]*\sasset=["']([^"']+)["']`)

// hasDocListParent reports whether assetID is a direct dot-namespaced child of some id passed as a
// <DocList> "asset" prop in this repo (e.g. "docs.faq.moves.a.1" is a direct child of
// "docs.faq.moves.a").
func (r *repoIndex) hasDocListParent(assetID string) bool {
	i := strings.LastIndex(assetID, ".")
	if i < 0 {
		return false
	}
	return r.docListParents[assetID[:i]]
}

var skipDir = map[string]bool{
	".git":         true,
	"node_modules": true,
	"build":        true,
	"dist":         true,
	".gradle":      true,
	".idea":        true,
	".claude":      true,
	"Pods":         true,
	"public":       true, // generated i18n json under public/static/i18n, not source usage
}

var relevantExtension = map[string]bool{
	".go":    true,
	".tpl":   true,
	".ts":    true,
	".tsx":   true,
	".js":    true,
	".jsx":   true,
	".kt":    true,
	".swift": true,
	".xml":   true,
	".html":  true,
}

// generatedFileMarker is the standard Go convention (also followed by this repo's own tools) for
// marking a file as machine-generated. get_translations' own "assets" command produces a file like
// this that lists every known asset id as a Go string constant, which would make every asset appear
// "used" by definition; skip generated files so only genuine application usage counts.
const generatedFileMarker = "Code generated"

// buildRepoIndex walks repoPath once and concatenates the contents of every source file with a
// relevant extension, skipping vendored/build/IDE directories and generated files.
func buildRepoIndex(platform, repoPath string) (*repoIndex, error) {
	if !isValidDir(repoPath) {
		return nil, fmt.Errorf("invalid directory: %s", repoPath)
	}

	var b strings.Builder
	docListParents := make(map[string]bool)
	localeIdentifiers := make(map[string]bool)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !relevantExtension[filepath.Ext(d.Name())] {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if isGeneratedFile(data) {
			return nil
		}
		for _, m := range docListAssetProp.FindAllSubmatch(data, -1) {
			docListParents[string(m[1])] = true
		}
		if platform == "backend" && filepath.Ext(path) == ".go" {
			collectLocaleIdentifiers(fset, path, data, localeIdentifiers)
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return &repoIndex{content: b.String(), docListParents: docListParents, localeIdentifiers: localeIdentifiers}, nil
}

// collectLocaleIdentifiers parses a Go source file and records every identifier referenced via a
// "locale.<Identifier>" selector expression (e.g. locale.EmailSubmitreminderSubject_Monthyear) into
// found. Backend code always references the locale package unqualified as "locale" (no import
// aliasing), so matching on that package name is sufficient. Parse errors are ignored: a file that
// doesn't parse contributes no identifiers rather than failing the whole scan.
func collectLocaleIdentifiers(fset *token.FileSet, path string, data []byte, found map[string]bool) {
	file, err := parser.ParseFile(fset, path, data, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "locale" {
			found[sel.Sel.Name] = true
		}
		return true
	})
}

// isGeneratedFile checks the first few lines of a file for the "Code generated ... DO NOT EDIT"
// marker convention.
func isGeneratedFile(data []byte) bool {
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}
	return strings.Contains(string(head), generatedFileMarker)
}

// searchCandidates returns the literal byte strings that would appear in source if assetID is in use
// on the given platform. hasChild indicates that some other asset is a dot-namespaced descendant of
// assetID (e.g. "nav.reports.averages.group" descends from "nav.reports.averages").
func searchCandidates(platform, assetID string, hasChild bool) []string {
	switch platform {
	case "android":
		mangled := androidMangle(assetID)
		return []string{"R.string." + mangled, "R.plurals." + mangled, "@string/" + mangled}
	case "ios":
		return []string{swiftMangle(assetID)}
	default:
		// backend, web, www reference the literal asset id as a quoted string. Requiring the
		// surrounding quote avoids false "used" matches when one asset id happens to be a prefix
		// of another (e.g. "nav.reports.monthly" is a prefix of "nav.reports.monthly.summary").
		candidates := []string{`"` + assetID + `"`, `'` + assetID + `'`}
		if hasChild && platform == "web" {
			// i18next's JSON export can't have assetID be both a leaf string and a parent object
			// in the same tree, so when a descendant asset also exists (e.g.
			// "nav.reports.averages.group"), the leaf value for assetID itself is nested under a
			// synthetic "0" key. Source code reaches it as assetID+".0" instead of the bare id.
			// This is purely an i18next JSON-export artifact: www (Hugo) consumes loco's flat,
			// dot-keyed YAML export (see hugo_yaml.go), which has no such nesting constraint.
			indexed := assetID + ".0"
			candidates = append(candidates, `"`+indexed+`"`, `'`+indexed+`'`)
		}
		return candidates
	}
}

// androidMangle reproduces aapt's resource-name mangling: any character that isn't [A-Za-z0-9_]
// becomes '_', except '%' which becomes the literal string "_0025" (as loco's own android export
// does), and a leading digit gets a '_' prefix since identifiers can't start with a digit.
func androidMangle(assetID string) string {
	var b strings.Builder
	for _, r := range assetID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r == '%':
			b.WriteString("_0025")
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out != "" && out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}

// swiftMangle reproduces Xcode's String Catalog symbol generation: the asset id is split on
// '.', '-', '_' and space, each resulting word is title-cased (after the first, which is
// lower-cased), and the pieces are joined with no separator.
func swiftMangle(assetID string) string {
	assetID = trailingPrintfParam.ReplaceAllString(assetID, "")
	chunks := splitOnDelimiters(assetID)

	var words []string
	for _, chunk := range chunks {
		words = append(words, splitCamelBoundaries(chunk)...)
	}
	if len(words) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(strings.ToLower(words[0]))
	for _, w := range words[1:] {
		if w == "" {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		b.WriteString(strings.ToLower(w[1:]))
	}
	return b.String()
}

var delimiterSplit = regexp.MustCompile(`[.\-_ ]+`)

func splitOnDelimiters(s string) []string {
	parts := delimiterSplit.Split(s, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

var camelBoundary = regexp.MustCompile(`[a-z][A-Z]|[0-9][A-Za-z]|[A-Za-z][0-9]`)

// splitCamelBoundaries breaks a chunk at lower->upper, digit->letter, and letter->digit boundaries,
// mirroring how the xcstrings tool further decomposes each dot/dash-separated component.
func splitCamelBoundaries(chunk string) []string {
	if chunk == "" {
		return nil
	}
	var indices []int
	for _, loc := range camelBoundary.FindAllStringIndex(chunk, -1) {
		indices = append(indices, loc[0]+1)
	}
	if len(indices) == 0 {
		return []string{chunk}
	}
	var out []string
	prev := 0
	for _, idx := range indices {
		out = append(out, chunk[prev:idx])
		prev = idx
	}
	out = append(out, chunk[prev:])
	return out
}
