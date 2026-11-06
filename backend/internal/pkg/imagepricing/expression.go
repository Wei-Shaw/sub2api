// Package imagepricing evaluates bounded, administrator-authored image prices.
// Expressions return total USD before the host applies its rate multiplier.
package imagepricing

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

const (
	MaxExpressions     = 64
	MaxExpressionBytes = 4096
	MaxImageCount      = 128
	MaxCostUSD         = 1_000_000
	// Usage logs persist the selected tier in a VARCHAR(50) column.
	MaxTierNameBytes = 50
	maxNodes         = 256
)

// Facts contains only owned scalar values, never prompts, images, or headers.
type Facts struct {
	Model       string `json:"model"`
	Resolution  string `json:"resolution,omitempty"`
	Quality     string `json:"quality,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Size        string `json:"size,omitempty"`
	ImageCount  int    `json:"image_count"`
	// RequestedCount is frozen at preflight; unlike ImageCount it does not change
	// when an upstream returns fewer images than requested.
	RequestedCount int `json:"requested_count"`
}

type Result struct {
	CostUSD        float64 `json:"cost_usd"`
	Tier           string  `json:"tier,omitempty"`
	ExpressionHash string  `json:"expression_hash"`
}

// Evaluator owns an immutable compiled program for one pricing snapshot. It has
// no global cache or background lifecycle; each evaluation owns its parameters.
type Evaluator struct {
	source  string
	program *vm.Program
}

func Compile(source string) (*Evaluator, error) {
	program, err := compile(source)
	if err != nil {
		return nil, err
	}
	return &Evaluator{source: source, program: program}, nil
}

// ValidateExpressions validates exact request-model keys and every expression.
// A nil or empty map opts out; callers preserve the distinction for updates.
func ValidateExpressions(expressions map[string]string) error {
	if len(expressions) > MaxExpressions {
		return fmt.Errorf("at most %d image price expressions are allowed", MaxExpressions)
	}
	for model, source := range expressions {
		if model == "" || len(model) > 200 || strings.TrimSpace(model) != model || strings.ContainsAny(model, "\r\n\x00*") {
			return fmt.Errorf("image price expressions require exact, non-empty model IDs (maximum 200 bytes)")
		}
		if err := validate(source, model); err != nil {
			return fmt.Errorf("image price expression for %q: %w", model, err)
		}
	}
	return nil
}

func Validate(source string) error { return validate(source, "") }

func validate(source, model string) error {
	program, err := compile(source)
	if err != nil {
		return err
	}
	// Exercise defaults, current common specifications and quantity boundaries.
	// Runtime validation remains mandatory for provider-specific values.
	for _, resolution := range []string{"", "1k", "1.5k", "2k", "4k"} {
		for _, quality := range []string{"", "auto", "low", "medium", "high", "standard"} {
			for _, count := range []int{0, 1, MaxImageCount} {
				_, err := run(program, source, Facts{Model: model, Resolution: resolution, Quality: quality, AspectRatio: "1:1", ImageCount: count, RequestedCount: max(1, count)})
				if err != nil {
					return fmt.Errorf("invalid price for resolution=%q quality=%q image_count=%d: %w", resolution, quality, count, err)
				}
			}
		}
	}
	return nil
}

func Evaluate(source string, facts Facts) (Result, error) {
	evaluator, err := Compile(source)
	if err != nil {
		return Result{}, err
	}
	return evaluator.Evaluate(facts)
}

func (e *Evaluator) Evaluate(facts Facts) (Result, error) {
	if facts.ImageCount < 0 || facts.ImageCount > MaxImageCount || facts.RequestedCount < 0 || facts.RequestedCount > MaxImageCount {
		return Result{}, fmt.Errorf("image_count must be between 0 and %d", MaxImageCount)
	}
	if len(facts.Model) > 200 || len(facts.Resolution) > 128 || len(facts.Quality) > 128 || len(facts.AspectRatio) > 128 || len(facts.Size) > 128 {
		return Result{}, fmt.Errorf("image pricing parameters exceed their size limit")
	}
	return run(e.program, e.source, facts)
}

func environment(facts Facts, tier *string) map[string]any {
	return map[string]any{
		"image_count": float64(facts.ImageCount),
		"param": func(key string) any {
			var value string
			switch key {
			case "model":
				value = facts.Model
			case "resolution":
				value = facts.Resolution
			case "quality":
				value = facts.Quality
			case "aspect_ratio":
				value = facts.AspectRatio
			case "size":
				value = facts.Size
			case "n":
				return float64(facts.RequestedCount)
			}
			if value == "" {
				return nil
			}
			return value
		},
		"fixed": func(value float64) float64 { return value },
		"tier":  func(name string, value float64) float64 { *tier = name; return value },
	}
}

func run(program *vm.Program, source string, facts Facts) (Result, error) {
	tier := ""
	value, err := expr.Run(program, environment(facts, &tier))
	if err != nil {
		return Result{}, fmt.Errorf("evaluate image price: %w", err)
	}
	cost, ok := value.(float64)
	if !ok || math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 || cost > MaxCostUSD {
		return Result{}, fmt.Errorf("image price must be finite and between 0 and %g USD", float64(MaxCostUSD))
	}
	return Result{CostUSD: cost, Tier: tier, ExpressionHash: fmt.Sprintf("%x", sha256.Sum256([]byte(source)))}, nil
}

func compile(source string) (*vm.Program, error) {
	if strings.TrimSpace(source) == "" || len(source) > MaxExpressionBytes {
		return nil, fmt.Errorf("image price expression must contain 1 to %d bytes", MaxExpressionBytes)
	}
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse image price: %w", err)
	}
	guard := &expressionGuard{}
	ast.Walk(&tree.Node, guard)
	if guard.err != nil {
		return nil, guard.err
	}
	tier := ""
	program, err := expr.Compile(source, expr.Env(environment(Facts{}, &tier)), expr.AsFloat64(), expr.DisableAllBuiltins(), expr.Optimize(false))
	if err != nil {
		return nil, fmt.Errorf("compile image price: %w", err)
	}

	return program, nil
}

type expressionGuard struct {
	nodes int
	err   error
}

func (g *expressionGuard) Visit(node *ast.Node) {
	if g.err != nil {
		return
	}
	g.nodes++
	if g.nodes > maxNodes {
		g.err = fmt.Errorf("image price expression exceeds %d nodes", maxNodes)
		return
	}
	switch n := (*node).(type) {
	case *ast.NilNode, *ast.BoolNode, *ast.IntegerNode, *ast.FloatNode, *ast.ConditionalNode:
	case *ast.StringNode:
		if len(n.Value) > 200 {
			g.err = fmt.Errorf("image price string literal exceeds 200 bytes")
		}
	case *ast.IdentifierNode:
		if n.Value != "image_count" && n.Value != "param" && n.Value != "tier" && n.Value != "fixed" {
			g.err = fmt.Errorf("unknown image price identifier %q", n.Value)
		}
	case *ast.UnaryNode:
		if n.Operator != "+" && n.Operator != "-" && n.Operator != "!" && n.Operator != "not" {
			g.err = fmt.Errorf("unsupported image price unary operator")
		}
	case *ast.BinaryNode:
		switch n.Operator {
		case "+", "-", "*", "/", "==", "!=", "<", "<=", ">", ">=", "&&", "||", "and", "or", "??":
		default:
			g.err = fmt.Errorf("unsupported image price operator %q", n.Operator)
		}
	case *ast.CallNode:
		callee, ok := n.Callee.(*ast.IdentifierNode)
		if !ok {
			g.err = fmt.Errorf("unsupported image price function")
			return
		}
		switch callee.Value {
		case "param":
			if len(n.Arguments) != 1 {
				g.err = fmt.Errorf("param requires one literal parameter name")
				return
			}
			key, ok := n.Arguments[0].(*ast.StringNode)
			if !ok {
				g.err = fmt.Errorf("param requires a literal parameter name")
				return
			}
			switch key.Value {
			case "model", "resolution", "quality", "aspect_ratio", "size", "n":
			default:
				g.err = fmt.Errorf("unsupported image pricing parameter %q", key.Value)
			}
		case "tier":
			if len(n.Arguments) != 2 {
				g.err = fmt.Errorf("tier requires a literal name and a price")
				return
			}
			if name, ok := n.Arguments[0].(*ast.StringNode); !ok {
				g.err = fmt.Errorf("tier name must be a string literal")
			} else if len(name.Value) > MaxTierNameBytes {
				g.err = fmt.Errorf("tier name exceeds %d bytes", MaxTierNameBytes)
			} else if strings.ContainsAny(name.Value, "\x00\r\n") {
				g.err = fmt.Errorf("tier name must not contain NUL or line breaks")
			}
		case "fixed":
			if len(n.Arguments) != 1 {
				g.err = fmt.Errorf("fixed requires one price")
			}
		default:
			g.err = fmt.Errorf("unsupported image price function %q", callee.Value)
		}
	default:
		g.err = fmt.Errorf("unsupported image price syntax %T", n)
	}
}
