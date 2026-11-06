package imagepricing

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

const grokPrice = `((param("resolution") == nil || param("resolution") == "1k")
  ? (param("quality") == "medium" ? tier("1k-medium", fixed(0.06)) : tier("1k-low", fixed(0.04)))
  : param("resolution") == "1.5k"
    ? (param("quality") == "medium" ? tier("1.5k-medium", fixed(0.07)) : tier("1.5k-low", fixed(0.05)))
    : param("resolution") == "2k"
      ? (param("quality") == "medium" ? tier("2k-medium", fixed(0.08)) : tier("2k-low", fixed(0.06)))
      : tier("unrecognized-resolution", fixed(0.08))) * image_count`

func TestGrokPricesAndActualCount(t *testing.T) {
	if err := ValidateExpressions(map[string]string{"grok-imagine-image-2.0": grokPrice}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		resolution, quality, tier string
		price                     float64
	}{
		{"1k", "low", "1k-low", .04}, {"1k", "medium", "1k-medium", .06},
		{"1.5k", "low", "1.5k-low", .05}, {"1.5k", "medium", "1.5k-medium", .07},
		{"2k", "low", "2k-low", .06}, {"2k", "medium", "2k-medium", .08},
		{"", "", "1k-low", .04}, {"", "auto", "1k-low", .04},
		{"4k", "medium", "unrecognized-resolution", .08},
	} {
		t.Run(tc.resolution+"/"+tc.quality, func(t *testing.T) {
			for _, count := range []int{0, 1, 2, 10} {
				result, err := Evaluate(grokPrice, Facts{Model: "grok-imagine-image-2.0", Resolution: tc.resolution, Quality: tc.quality, ImageCount: count, RequestedCount: 10})
				if err != nil {
					t.Fatal(err)
				}
				if math.Abs(result.CostUSD-tc.price*float64(count)) > 1e-12 || result.Tier != tc.tier || len(result.ExpressionHash) != 64 {
					t.Fatalf("unexpected result: %+v", result)
				}
			}
		})
	}
}

func TestRequestCountDoesNotBecomeActualCount(t *testing.T) {
	source := `param("n") == 3 && param("aspect_ratio") == "16:9" ? fixed(0.07) * image_count : 0`
	result, err := Evaluate(source, Facts{RequestedCount: 3, ImageCount: 2, AspectRatio: "16:9"})
	if err != nil || math.Abs(result.CostUSD-.14) > 1e-12 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestProviderSizePricing(t *testing.T) {
	result, err := Evaluate(`param("size") == "1024x1536" ? fixed(0.08) * image_count : fixed(0.04) * image_count`, Facts{Size: "1024x1536", ImageCount: 2, RequestedCount: 2})
	if err != nil || math.Abs(result.CostUSD-.16) > 1e-12 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestInvalidExpressionsFailClosed(t *testing.T) {
	for _, source := range []string{
		"", "fixed(", "-1", "1 / 0", "0 / 0", "1000001", `param("prompt")`,
		`param("resolution" + "")`, `param("n").value`, `env`, `now()`, `sum([1,2])`,
		`map(1..100, #)`, `let p = 1; p`, `2 ** 99`, `tier(param("model"), 1)`,
		strings.Repeat("1+", 200) + "1", strings.Repeat(" ", MaxExpressionBytes+1),
		`tier("` + strings.Repeat("x", MaxTierNameBytes+1) + `", 0.04)`,
		`tier("bad\u0000", 0.04)`, `tier("bad\nlabel", 0.04)`,
	} {
		t.Run(source[:min(35, len(source))], func(t *testing.T) {
			if err := Validate(source); err == nil {
				t.Fatalf("accepted unsafe/invalid expression: %q", source)
			}
		})
	}
	// A branch not exercised by the save-time sample grid still fails at runtime.
	if _, err := Evaluate(`param("resolution") == "custom" ? -1 : 0.04`, Facts{Resolution: "custom", ImageCount: 1}); err == nil {
		t.Fatal("negative runtime price accepted")
	}
	if _, err := Evaluate("0.04", Facts{ImageCount: MaxImageCount + 1}); err == nil {
		t.Fatal("unbounded count accepted")
	}
	if _, err := Evaluate("0.04", Facts{RequestedCount: -1}); err == nil {
		t.Fatal("negative request count accepted")
	}
}

func TestExactModelConfigurationLimits(t *testing.T) {
	for _, model := range []string{"", " model", "model ", "model*", "model\n", strings.Repeat("m", 201)} {
		if ValidateExpressions(map[string]string{model: "0.04"}) == nil {
			t.Fatalf("accepted key %q", model)
		}
	}
	expressions := map[string]string{}
	for i := 0; i <= MaxExpressions; i++ {
		expressions[fmt.Sprint(i)] = "0.04"
	}
	if ValidateExpressions(expressions) == nil {
		t.Fatal("accepted too many expressions")
	}
	if err := ValidateExpressions(map[string]string{"Unknown-NewModel": "0.04", "unknown-newmodel": "0.05"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExpressions(nil); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentProgramsHaveIsolatedFactsAndTiers(t *testing.T) {
	evaluator, err := Compile(grokPrice)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 1; i <= 16; i++ {
		wg.Add(1)
		go func(count int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				quality := "low"
				price := .04
				tier := "1k-low"
				if count%2 == 0 {
					quality = "medium"
					price = .06
					tier = "1k-medium"
				}
				r, err := evaluator.Evaluate(Facts{Quality: quality, ImageCount: count})
				if err != nil || math.Abs(r.CostUSD-price*float64(count)) > 1e-12 || r.Tier != tier {
					t.Errorf("shared evaluation state: result=%+v err=%v", r, err)
				}
			}
		}(i)
	}
	wg.Wait()
}
