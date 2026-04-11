package rename

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

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
		Metadata:     cloneStringMap(sample.Metadata),
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

func slug(value string) string {
	value = core.NormalizeTagName(value)
	return strings.ReplaceAll(value, " ", "-")
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
