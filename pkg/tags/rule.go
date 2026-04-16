package tags

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/darkliquid/zounds/pkg/core"
)

const ruleTaggerVersion = "0.2.0"

type RuleDefinition struct {
	Tag        string  `json:"tag"`
	Expr       string  `json:"expr"`
	Confidence float64 `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"`
}

type RuleConfig struct {
	Rules []RuleDefinition `json:"rules"`
}

type compiledRule struct {
	definition RuleDefinition
	program    *vm.Program
}

type RuleTagger struct {
	rules []compiledRule
}

type ruleSampleEnv struct {
	Path         string
	RelativePath string
	FileName     string
	Extension    string
	Format       string
	SizeBytes    int64
}

type ruleEnv struct {
	Sample     ruleSampleEnv
	Metrics    map[string]float64
	Attributes map[string]string
}

func NewRuleTaggerWithRules(definitions []RuleDefinition) (RuleTagger, error) {
	compiled := make([]compiledRule, 0, len(definitions))
	for _, definition := range definitions {
		rule, err := compileRule(definition)
		if err != nil {
			return RuleTagger{}, err
		}
		compiled = append(compiled, rule)
	}
	return RuleTagger{rules: compiled}, nil
}

func NewRuleTaggerFromFile(path string) (RuleTagger, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return RuleTagger{}, fmt.Errorf("read rule config %q: %w", path, err)
	}
	definitions, err := parseRuleDefinitions(body)
	if err != nil {
		return RuleTagger{}, fmt.Errorf("parse rule config %q: %w", path, err)
	}
	return NewRuleTaggerWithRules(definitions)
}

func (RuleTagger) Name() string {
	return "rules"
}

func (RuleTagger) Version() string {
	return ruleTaggerVersion
}

func (t RuleTagger) Tags(_ context.Context, sample core.Sample, result core.AnalysisResult) ([]core.Tag, error) {
	env := ruleEnv{
		Sample: ruleSampleEnv{
			Path:         sample.Path,
			RelativePath: sample.RelativePath,
			FileName:     sample.FileName,
			Extension:    sample.Extension,
			Format:       string(sample.Format),
			SizeBytes:    sample.SizeBytes,
		},
		Metrics:    result.Metrics,
		Attributes: result.Attributes,
	}

	tags := make([]core.Tag, 0, len(t.rules))
	for _, rule := range t.rules {
		matched, err := expr.Run(rule.program, env)
		if err != nil {
			return nil, fmt.Errorf("evaluate rule %q: %w", rule.definition.Tag, err)
		}
		ok, _ := matched.(bool)
		if !ok {
			continue
		}
		tags = append(tags, newRuleTag(rule.definition))
	}

	return uniqueTags(tags), nil
}

func parseRuleDefinitions(body []byte) ([]RuleDefinition, error) {
	var config RuleConfig
	if err := json.Unmarshal(body, &config); err == nil && len(config.Rules) > 0 {
		return config.Rules, nil
	}

	var definitions []RuleDefinition
	if err := json.Unmarshal(body, &definitions); err != nil {
		return nil, err
	}
	return definitions, nil
}

func compileRule(definition RuleDefinition) (compiledRule, error) {
	tag := core.NormalizeTagName(definition.Tag)
	if tag == "" {
		return compiledRule{}, fmt.Errorf("rule tag is required")
	}
	expression := strings.TrimSpace(definition.Expr)
	if expression == "" {
		return compiledRule{}, fmt.Errorf("rule %q expression is required", tag)
	}
	if definition.Confidence <= 0 {
		definition.Confidence = 0.7
	}
	if strings.TrimSpace(definition.Source) == "" {
		definition.Source = "rules"
	}
	definition.Tag = tag

	program, err := expr.Compile(expression, expr.Env(ruleEnv{}), expr.AsBool())
	if err != nil {
		return compiledRule{}, fmt.Errorf("compile rule %q: %w", tag, err)
	}
	return compiledRule{
		definition: definition,
		program:    program,
	}, nil
}

func newRuleTag(definition RuleDefinition) core.Tag {
	return core.Tag{
		Name:           definition.Tag,
		NormalizedName: definition.Tag,
		Source:         definition.Source,
		Confidence:     definition.Confidence,
	}
}

func uniqueTags(tags []core.Tag) []core.Tag {
	seen := make(map[string]struct{}, len(tags))
	out := make([]core.Tag, 0, len(tags))
	for _, tag := range tags {
		if tag.NormalizedName == "" {
			continue
		}
		if _, ok := seen[tag.NormalizedName]; ok {
			continue
		}
		seen[tag.NormalizedName] = struct{}{}
		out = append(out, tag)
	}
	return out
}
