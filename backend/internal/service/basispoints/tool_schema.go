package basispoints

import (
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"strconv"
	"strings"
)

// Compile once per request, preserve json.Number, and never resolve external
// schemas. Validation errors must not echo private argument values.
func compileToolSchema(parameters object) (*jsonschema.Schema, error) {
	if parameters == nil {
		return nil, nil
	}
	raw, err := json.Marshal(parameters)
	if err != nil || len(raw) > 1<<20 {
		return nil, fmt.Errorf("basispoints tool schema exceeds 1 MiB")
	}
	if err := validateToolNumberBudget(parameters); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(localSchemaLoader{})
	const schemaURL = "https://basispoints.invalid/client-tool.json"
	if err := compiler.AddResource(schemaURL, parameters); err != nil {
		return nil, fmt.Errorf("basispoints tool schema is invalid")
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("basispoints tool schema is invalid or references an external resource")
	}
	return schema, nil
}

// The schema library expands JSON exponents into big.Rat. Bound expansion
// before compilation and validation so tiny exponent strings cannot allocate
// unbounded integers. Ordinary large IDs remain exact.
func validateToolNumberBudget(value any) error {
	budget := 65536
	var visit func(any, int) error
	visit = func(value any, depth int) error {
		if depth > 128 {
			return fmt.Errorf("basispoints tool schema or arguments exceed the nesting limit")
		}
		switch v := value.(type) {
		case json.Number:
			raw := v.String()
			if len(raw) > 4096 {
				return fmt.Errorf("basispoints tool number exceeds the precision limit")
			}
			digits := len(raw)
			if at := strings.IndexAny(raw, "eE"); at >= 0 {
				exponent, err := strconv.Atoi(raw[at+1:])
				if err != nil || exponent > 4096 || exponent < -4096 {
					return fmt.Errorf("basispoints tool number exceeds the exponent limit")
				}
				if exponent < 0 {
					exponent = -exponent
				}
				digits += exponent
			}
			budget -= digits
			if budget < 0 {
				return fmt.Errorf("basispoints tool numbers exceed the validation budget")
			}
		case object:
			for _, child := range v {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(value, 0)
}

type toolArgumentsSchemaError struct{}

func (toolArgumentsSchemaError) Error() string {
	return "basispoints function arguments do not satisfy the declared tool schema or validation resource limits"
}
