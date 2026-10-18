package qualitytrace

// ModelTrace scoring, ported from xqy2006/ModelTrace (MIT).
// See data/README.md for the pinned source and license.
import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"unicode"
)

//go:embed data/unified_bank.json
var modelTraceBankJSON []byte

//go:embed data/LICENSE
var modelTraceLicense string

type traceFeatures struct {
	Mean         []float64     `json:"feature_mean"`
	Scale        []float64     `json:"feature_scale"`
	Basis        [][]float64   `json:"nuisance_basis"`
	Centroids    [][]float64   `json:"centroids"`
	Environments [][][]float64 `json:"environment_centroids"`
	Weight       float64       `json:"weight"`
}

type traceBankModel struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Family      string    `json:"family"`
	FamilyName  string    `json:"family_name"`
	Counts      []float64 `json:"counts"`
}

type traceBank struct {
	Models []traceBankModel `json:"models"`
	Robust struct {
		Hellinger traceFeatures `json:"hellinger"`
		Ordered   traceFeatures `json:"ordered_blocks"`
	} `json:"robust"`
	Calibration map[string]struct {
		Beta float64 `json:"beta"`
	} `json:"calibration"`
}

func decodeBank() traceBank {
	var bank traceBank
	if err := json.Unmarshal(modelTraceBankJSON, &bank); err != nil {
		panic("invalid embedded ModelTrace reference bank")
	}
	return bank
}

var modelTraceBank = decodeBank()
var traceBankRevision = fmt.Sprintf("sha256:%x", sha256.Sum256(modelTraceBankJSON))

type traceOutput struct {
	Text          string `json:"text"`
	ExpectedCount int    `json:"expected_count"`
}

type traceDiagnostic struct {
	Index   int  `json:"index"`
	Parsed  int  `json:"parsed_numbers"`
	Minimum int  `json:"minimum_numbers"`
	Valid   bool `json:"accepted"`
}

type traceCandidate struct {
	Model       string  `json:"model"`
	DisplayName string  `json:"display_name"`
	Family      string  `json:"family"`
	FamilyName  string  `json:"family_name"`
	Probability float64 `json:"probability"`
	Similarity  float64 `json:"profile_similarity"`
	Score       float64 `json:"score"`
}

type traceFamily struct {
	Family      string  `json:"family"`
	DisplayName string  `json:"display_name"`
	Probability float64 `json:"probability"`
}

type traceAttribution struct {
	Prediction        string            `json:"prediction"`
	Probability       float64           `json:"probability"`
	Used              int               `json:"used_outputs"`
	FamilyPrediction  string            `json:"family_prediction_name"`
	FamilyProbability float64           `json:"family_probability"`
	Results           []traceCandidate  `json:"results"`
	Families          []traceFamily     `json:"family_probabilities"`
	Diagnostics       []traceDiagnostic `json:"diagnostics"`
}

var traceDigits = regexp.MustCompile(`[0-9]+`)

func traceMinimumNumbers(expected int) int {
	return max(80, int(math.Ceil(float64(expected)*0.55)))
}

// Keep the longest run, ignoring introductory counts and numbers in prose.
// ASCII digits and Unicode letters match the upstream browser implementation.
func traceParseNumbers(text string) []int {
	var best, current []int
	end := 0
	for _, span := range traceDigits.FindAllStringIndex(text, -1) {
		for _, r := range text[end:span[0]] {
			if unicode.IsLetter(r) {
				if len(current) > len(best) {
					best = current
				}
				current = nil
				break
			}
		}
		if n, err := strconv.Atoi(text[span[0]:span[1]]); err == nil && n >= 1 && n <= 355 {
			current = append(current, n)
		}
		end = span[1]
	}
	if len(current) > len(best) {
		best = current
	}
	return best
}

func traceDot(a, b []float64) float64 {
	sum := 0.0
	for i, v := range a {
		sum += v * b[i]
	}
	return sum
}

func traceNormalize(a []float64) []float64 {
	scale := math.Max(math.Sqrt(traceDot(a, a)), 1e-12)
	for i := range a {
		a[i] /= scale
	}
	return a
}

func traceStandardize(a []float64) []float64 {
	mean := 0.0
	for _, v := range a {
		mean += v / float64(len(a))
	}
	variance := 0.0
	for _, v := range a {
		variance += (v - mean) * (v - mean) / float64(len(a))
	}
	scale := math.Max(math.Sqrt(variance), 1e-12)
	for i := range a {
		a[i] = (a[i] - mean) / scale
	}
	return a
}

func traceProject(a []float64, basis [][]float64) []float64 {
	for _, b := range basis {
		projection := traceDot(a, b)
		for i := range a {
			a[i] -= projection * b[i]
		}
	}
	return a
}

func traceFeatureScale(a []float64, f traceFeatures) []float64 {
	for i := range a {
		a[i] = (a[i] - f.Mean[i]) / f.Scale[i]
	}
	return a
}

func traceCentroidScores(a []float64, centroids [][]float64) []float64 {
	scores := make([]float64, len(centroids))
	for i, c := range centroids {
		scores[i] = traceDot(a, c)
	}
	return traceStandardize(scores)
}

func traceCounts(numbers []int) []float64 {
	counts := make([]float64, 355)
	for _, n := range numbers {
		counts[n-1]++
	}
	return counts
}

func traceSmoothFeature(counts []float64) []float64 {
	total := float64(len(counts)) * 0.5
	for _, n := range counts {
		total += n
	}
	for i, n := range counts {
		counts[i] = math.Sqrt((n + 0.5) / total)
	}
	return counts
}

func traceScores(numbers []int, bank traceBank) []float64 {
	h := bank.Robust.Hellinger
	feature := traceFeatureScale(traceSmoothFeature(traceCounts(numbers)), h)
	marginal := traceCentroidScores(traceNormalize(traceProject(feature, h.Basis)), h.Centroids)
	o := bank.Robust.Ordered
	if o.Weight == 0 {
		return marginal
	}
	ordered := make([]float64, 0, 74)
	start := 0
	for i := 0; i < 4; i++ {
		size := len(numbers) / 4
		if i < len(numbers)%4 {
			size++
		}
		bins := make([]float64, 16)
		for _, n := range numbers[start : start+size] {
			bins[min(15, (n-1)*16/355)]++
		}
		ordered = append(ordered, traceSmoothFeature(bins)...)
		start += size
	}
	lastDigits := make([]float64, 10)
	for _, n := range numbers {
		lastDigits[n%10]++
	}
	ordered = append(ordered, traceSmoothFeature(lastDigits)...)
	ordered = traceFeatureScale(ordered, o)
	unit := traceNormalize(append([]float64(nil), ordered...))
	template := make([]float64, len(o.Centroids))
	for i := range template {
		template[i] = math.Inf(-1)
		for _, env := range o.Environments {
			template[i] = math.Max(template[i], traceDot(unit, env[i]))
		}
	}
	traceStandardize(template)
	nuisance := traceCentroidScores(traceNormalize(traceProject(ordered, o.Basis)), o.Centroids)
	for i := range template {
		template[i] = 0.5*template[i] + 0.5*nuisance[i]
	}
	traceStandardize(template)
	for i := range marginal {
		marginal[i] = (1-o.Weight)*marginal[i] + o.Weight*template[i]
	}
	return marginal
}

func traceSimilarity(left, right []float64) float64 {
	lt, rt := 0.0, 0.5*355
	for i := range left {
		lt += left[i]
		rt += right[i]
	}
	js := 0.0
	for i := range left {
		p, q := left[i]/lt, (right[i]+0.5)/rt
		mid := (p + q) / 2
		if p > 0 {
			js += p * math.Log(p/mid) / 2
		}
		js += q * math.Log(q/mid) / 2
	}
	return 1 - math.Sqrt(math.Max(0, js)/math.Log(2))
}

func analyzeModelTrace(outputs []traceOutput) (traceAttribution, error) {
	bank := modelTraceBank
	r := traceAttribution{}
	scores, pooled := make([]float64, len(bank.Models)), make([]float64, 355)
	for i, output := range outputs {
		numbers := traceParseNumbers(output.Text)
		minimum := traceMinimumNumbers(output.ExpectedCount)
		valid := len(numbers) >= minimum
		r.Diagnostics = append(r.Diagnostics, traceDiagnostic{i, len(numbers), minimum, valid})
		if !valid {
			continue
		}
		r.Used++
		for j, score := range traceScores(numbers, bank) {
			scores[j] += score
		}
		for _, n := range numbers {
			pooled[n-1]++
		}
	}
	if r.Used == 0 {
		return r, fmt.Errorf("未采集到有效数字序列")
	}
	beta := bank.Calibration[strconv.Itoa(min(r.Used, 3))].Beta
	maximum := math.Inf(-1)
	for i := range scores {
		scores[i] /= float64(r.Used)
		maximum = math.Max(maximum, beta*scores[i])
	}
	total := 0.0
	probabilities := make([]float64, len(scores))
	for i, s := range scores {
		probabilities[i] = math.Exp(beta*s - maximum)
		total += probabilities[i]
	}
	families := map[string]int{}
	for i, m := range bank.Models {
		probability := probabilities[i] / total
		r.Results = append(r.Results, traceCandidate{m.ID, m.DisplayName, m.Family, m.FamilyName, probability, traceSimilarity(pooled, m.Counts), scores[i]})
		if _, ok := families[m.Family]; !ok {
			families[m.Family] = len(r.Families)
			r.Families = append(r.Families, traceFamily{Family: m.Family, DisplayName: m.FamilyName})
		}
		r.Families[families[m.Family]].Probability += probability
	}
	sort.SliceStable(r.Results, func(i, j int) bool { return r.Results[i].Probability > r.Results[j].Probability })
	r.Prediction, r.Probability = r.Results[0].Model, r.Results[0].Probability
	for _, f := range r.Families {
		if f.Probability > r.FamilyProbability {
			r.FamilyPrediction, r.FamilyProbability = f.DisplayName, f.Probability
		}
	}
	return r, nil
}
