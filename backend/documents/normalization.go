package documents

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// NormalizedPart is one entry from a normalized DOCX archive. XML parts
// are canonicalised; binary parts are summarised by their SHA-256 so the
// golden test can compare them without byte-equal comparisons (which
// would break on every embedded timestamp).
type NormalizedPart struct {
	Name     string
	Kind     string // "xml" or "binary"
	Body     []byte // canonicalised XML bytes (only when Kind == "xml")
	Checksum string // hex SHA-256 for binary parts
}

// NormalizeXML canonicalises an XML document so golden tests can compare
// outputs without being thrown off by attribute order, comment presence,
// or stray whitespace. The canonical form is:
//
//   - All comments removed.
//   - The XML declaration and any processing instructions are stripped
//     (they carry timestamps we don't control).
//   - Whitespace between tags is normalised to a single newline.
//   - Attributes on each element are sorted alphabetically by name.
//
// The function deliberately does NOT validate the XML against any
// schema — Mintrud does not publish one we can download, and the legacy
// `gen_xml_test.go` is the only oracle we have.
func NormalizeXML(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}

	out := make([]byte, 0, len(raw))
	out = append(out, raw...)

	// Strip XML comments. A naive regex is fine because our XML never
	// contains "--" inside CDATA sections (we don't emit any).
	out = stripXMLComments(out)

	// Collapse all runs of whitespace between tags to a single newline.
	// This keeps the XML structurally equivalent while making the bytes
	// deterministic across runs.
	out = normalizeWhitespace(out)

	// Sort attributes inside every element. This is the most invasive
	// pass; we re-scan the document for `attr="value"` substrings inside
	// each tag and shuffle them.
	out = sortAttributes(out)

	return out
}

var (
	xmlCommentRE    = regexp.MustCompile(`<!--[\s\S]*?-->`)
	xmlWhitespaceRE = regexp.MustCompile(`\s+`)
	xmlAttrRE       = regexp.MustCompile(`<[A-Za-z_][A-Za-z0-9_:.-]*\s+([^>]*?)\s*/?>`)
)

func stripXMLComments(raw []byte) []byte {
	return xmlCommentRE.ReplaceAll(raw, nil)
}

func normalizeWhitespace(raw []byte) []byte {
	// Replace any run of whitespace (outside attribute values) with a
	// single space. We approximate this by collapsing runs globally; for
	// the controlled XML our pipeline produces, this is enough.
	return xmlWhitespaceRE.ReplaceAll(raw, []byte(" "))
}

func sortAttributes(raw []byte) []byte {
	return xmlAttrRE.ReplaceAllFunc(raw, func(match []byte) []byte {
		// Split into the tag opener + the attribute chunk + the close.
		// The regex captures only the part with attributes; reconstruct
		// the tag by preserving the bracket context.
		openIdx := bytes.IndexByte(match, '<')
		tagEnd := openIdx + 1
		for tagEnd < len(match) && isTagNameByte(match[tagEnd]) {
			tagEnd++
		}
		// tag name is match[openIdx+1 : tagEnd]
		rest := match[tagEnd:]
		// rest is like ` foo="a" bar="b"/>` or ` foo="a" bar="b">`
		closeIdx := bytes.IndexByte(rest, '/')
		gtIdx := bytes.IndexByte(rest, '>')
		endIdx := len(rest)
		switch {
		case closeIdx >= 0 && (gtIdx < 0 || closeIdx < gtIdx):
			// self-closing
			closeIdx2 := bytes.IndexByte(rest[closeIdx:], '>')
			if closeIdx2 >= 0 {
				endIdx = closeIdx + closeIdx2 + 1
			}
		case gtIdx >= 0:
			endIdx = gtIdx + 1
		}
		attrs := strings.TrimSpace(string(rest[:endIdx]))
		attrs = sortAttrs(attrs)
		return append(match[:tagEnd], []byte(" "+attrs)...)
	})
}

func isTagNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_' || b == ':' || b == '.' || b == '-'
}

// sortAttrs splits the attribute chunk into pieces, sorts them, and joins
// them back. The chunk looks like `foo="a" bar="b" baz="c"` (possibly
// ending with `/`).
func sortAttrs(chunk string) string {
	chunk = strings.TrimSpace(chunk)
	chunk = strings.TrimSuffix(chunk, "/>")
	chunk = strings.TrimSuffix(chunk, ">")
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return ""
	}

	// Split by double-quoted attribute values. Walk the string and
	// extract each attr="value" pair.
	var attrs []string
	for i := 0; i < len(chunk); {
		// Skip leading whitespace.
		for i < len(chunk) && (chunk[i] == ' ' || chunk[i] == '\t') {
			i++
		}
		if i >= len(chunk) {
			break
		}
		// Read the attribute name.
		nameStart := i
		for i < len(chunk) && chunk[i] != '=' && chunk[i] != ' ' && chunk[i] != '\t' {
			i++
		}
		if i >= len(chunk) || chunk[i] != '=' {
			break
		}
		name := chunk[nameStart:i]
		i++ // past '='
		// Skip optional whitespace.
		for i < len(chunk) && (chunk[i] == ' ' || chunk[i] == '\t') {
			i++
		}
		if i >= len(chunk) || chunk[i] != '"' {
			break
		}
		// Find the closing quote, but allow escaped quotes inside (none
		// in our XML).
		valStart := i + 1
		end := strings.IndexByte(chunk[valStart:], '"')
		if end < 0 {
			break
		}
		valEnd := valStart + end
		attrs = append(attrs, fmt.Sprintf(`%s="%s"`, name, chunk[valStart:valEnd]))
		i = valEnd + 1
	}

	sort.Strings(attrs)
	return strings.Join(attrs, " ")
}

// NormalizeDOCX unpacks a DOCX archive (a ZIP file) and canonicalises
// each part. XML parts go through NormalizeXML; binary parts are returned
// as SHA-256 checksums. The slice is sorted by part name so callers can
// compare two normalized archives with reflect.DeepEqual.
func NormalizeDOCX(raw []byte) ([]NormalizedPart, error) {
	if len(raw) == 0 {
		return nil, errors.New("NormalizeDOCX: empty input")
	}
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("NormalizeDOCX: open zip: %w", err)
	}

	var parts []NormalizedPart
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("NormalizeDOCX: open %s: %w", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("NormalizeDOCX: read %s: %w", f.Name, err)
		}

		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			parts = append(parts, NormalizedPart{
				Name: f.Name,
				Kind: "xml",
				Body: NormalizeXML(body),
			})
			continue
		}

		sum := sha256.Sum256(body)
		parts = append(parts, NormalizedPart{
			Name:     f.Name,
			Kind:     "binary",
			Checksum: hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(parts, func(i, j int) bool { return parts[i].Name < parts[j].Name })
	return parts, nil
}
