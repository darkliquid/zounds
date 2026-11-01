package rename

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/darkliquid/zounds/pkg/core"
)

type TemplateData struct {
	ID           int64
	Name         string
	Stem         string
	Extension    string
	Format       string
	Path         string
	RelativePath string
	SourceRoot   string
	SizeBytes    int64
	Tags         []string
	TagsBySource map[string][]string
	Attributes   map[string]string
	Metadata     map[string]string
}

type RenameOp struct {
	Sample      core.Sample
	Destination string
}

func BuildTemplateData(sample core.Sample, tags []core.Tag, attributes map[string]string) TemplateData {
	tagNames := make([]string, 0, len(tags))
	bySource := make(map[string][]string)
	for _, tag := range tags {
		name := tag.NormalizedName
		if name == "" {
			name = core.NormalizeTagName(tag.Name)
		}
		if name == "" {
			continue
		}
		tagNames = append(tagNames, name)
		if tag.Source != "" {
			bySource[tag.Source] = append(bySource[tag.Source], name)
		}
	}

	slices.Sort(tagNames)
	for source := range bySource {
		slices.Sort(bySource[source])
	}

	metadata := cloneStringMap(sample.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	for k, v := range attributes {
		if strings.HasPrefix(k, "embedded.") {
			shortKey := strings.TrimPrefix(k, "embedded.")
			if _, exists := metadata[shortKey]; !exists {
				metadata[shortKey] = v
			}
		}
	}

	return TemplateData{
		ID:           sample.ID,
		Name:         sample.FileName,
		Stem:         strings.TrimSuffix(sample.FileName, filepath.Ext(sample.FileName)),
		Extension:    sample.Extension,
		Format:       string(sample.Format),
		Path:         sample.Path,
		RelativePath: sample.RelativePath,
		SourceRoot:   sample.SourceRoot,
		SizeBytes:    sample.SizeBytes,
		Tags:         tagNames,
		TagsBySource: bySource,
		Attributes:   cloneStringMap(attributes),
		Metadata:     metadata,
	}
}

func RenderTemplate(text string, data TemplateData) (string, error) {
	tmpl, err := template.New("rename").Funcs(template.FuncMap{
		"join":  strings.Join,
		"slug":  slug,
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}).Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse rename template: %w", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("execute rename template: %w", err)
	}
	return out.String(), nil
}

func ResolveTarget(sample core.Sample, rendered string) (string, error) {
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return "", fmt.Errorf("rename template rendered an empty path")
	}
	if strings.ContainsRune(rendered, '\x00') {
		return "", fmt.Errorf("rename template rendered NUL byte")
	}
	if strings.HasSuffix(rendered, "/") || strings.HasSuffix(rendered, `\`) {
		return "", fmt.Errorf("rename template rendered directory path %q without filename", rendered)
	}

	if sample.Extension != "" {
		wantExt := "." + sample.Extension
		if !strings.EqualFold(filepath.Ext(rendered), wantExt) {
			rendered += wantExt
		}
	}

	var dest string
	if filepath.IsAbs(rendered) {
		dest = filepath.Clean(rendered)
	} else {
		if strings.TrimSpace(sample.SourceRoot) == "" {
			return "", fmt.Errorf("sample %q has empty SourceRoot for relative rename target %q", sample.Path, rendered)
		}
		rootClean := filepath.Clean(sample.SourceRoot)
		dest = filepath.Clean(filepath.Join(rootClean, rendered))
		rel, err := filepath.Rel(rootClean, dest)
		if err != nil {
			return "", fmt.Errorf("resolve relative rename target %q: %w", rendered, err)
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("rename target %q escapes source root %q", rendered, rootClean)
		}
	}

	base := filepath.Base(dest)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return "", fmt.Errorf("rename target %q does not name a valid file", dest)
	}

	return dest, nil
}

func ValidatePlan(ops []RenameOp) error {
	seenTargets := make(map[string]string, len(ops))
	for _, op := range ops {
		dest := filepath.Clean(op.Destination)
		src := filepath.Clean(op.Sample.Path)
		if prevSrc, exists := seenTargets[dest]; exists && prevSrc != src {
			return fmt.Errorf("batch rename target collision: both %q and %q map to %q", prevSrc, src, dest)
		}
		seenTargets[dest] = src

		if src == dest {
			continue
		}
		if _, err := os.Lstat(dest); err == nil {
			return fmt.Errorf("rename destination %q already exists (refusing to overwrite from %q)", dest, src)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check rename destination %q: %w", dest, err)
		}
	}
	return nil
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsControl(r):
			b.WriteByte(' ')
		case strings.ContainsRune(` _-:/\<>|"?*`, r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}

	tokens := strings.Fields(b.String())
	cleaned := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		for strings.Contains(tok, "..") {
			tok = strings.ReplaceAll(tok, "..", ".")
		}
		tok = strings.Trim(tok, ".")
		if tok != "" {
			cleaned = append(cleaned, tok)
		}
	}
	return strings.Join(cleaned, "-")
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
