package backend

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrTooManyReferenceImages = errors.New("too many reference images")
var ErrInvalidOutputCount = errors.New("invalid output image count")
var ErrInvalidImageParameter = errors.New("invalid image parameter")

type EnumCapability struct {
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

type IntegerCapability struct {
	Min     int `json:"min"`
	Max     int `json:"max"`
	Default int `json:"default"`
}

type ImageModelCapability struct {
	MaxReferenceImages int                `json:"max_reference_images"`
	MaxOutputImages    int                `json:"max_output_images"`
	Sizes              *EnumCapability    `json:"sizes,omitempty"`
	AspectRatios       *EnumCapability    `json:"aspect_ratios,omitempty"`
	Resolutions        *EnumCapability    `json:"resolutions,omitempty"`
	Quality            *EnumCapability    `json:"quality,omitempty"`
	OutputFormats      *EnumCapability    `json:"output_formats,omitempty"`
	OutputCompression  *IntegerCapability `json:"output_compression,omitempty"`
	Background         *EnumCapability    `json:"background,omitempty"`
	InputFidelity      *EnumCapability    `json:"input_fidelity,omitempty"`
}

var imageModelCapabilities = map[string]ImageModelCapability{
	"gpt-image-2":            gptImageCapability(),
	"gpt-image-1":            gptImageCapability(),
	"gemini-2.5-flash-image": geminiImageCapability(),
}

func gptImageCapability() ImageModelCapability {
	return ImageModelCapability{
		MaxReferenceImages: 16,
		MaxOutputImages:    10,
		Sizes:              enumCapability("1024x1024", "1024x1024", "1536x1024", "1024x1536"),
		Quality:            enumCapability("auto", "auto", "low", "medium", "high"),
		OutputFormats:      enumCapability("png", "png", "jpeg", "webp"),
		OutputCompression:  &IntegerCapability{Min: 0, Max: 100, Default: 100},
		Background:         enumCapability("auto", "auto", "transparent", "opaque"),
		InputFidelity:      enumCapability("high", "low", "high"),
	}
}

func geminiImageCapability() ImageModelCapability {
	return ImageModelCapability{
		MaxReferenceImages: 10,
		MaxOutputImages:    4,
		AspectRatios:       enumCapability("1:1", "1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"),
		Resolutions:        enumCapability("1K", "1K", "2K", "4K"),
	}
}

func enumCapability(defaultValue string, values ...string) *EnumCapability {
	return &EnumCapability{Values: values, Default: defaultValue}
}

func configuredImageModelCapability(modelName string) (ImageModelCapability, bool) {
	capability, ok := imageModelCapabilities[strings.TrimSpace(modelName)]
	return capability, ok
}

func imageModelCapability(modelName string) ImageModelCapability {
	if capability, ok := configuredImageModelCapability(modelName); ok {
		return capability
	}
	return ImageModelCapability{MaxReferenceImages: 1, MaxOutputImages: 1}
}

func normalizeImageParameters(req *GenerateRequest) error {
	capability, configured := configuredImageModelCapability(req.Model)
	if !configured {
		if name, value := firstAdvancedImageParameter(*req); value != "" {
			return invalidImageParameter(req.Model, name, value, "no advanced parameters")
		}
		req.Size = strings.TrimSpace(req.Size)
		if req.Size == "" {
			req.Size = defaultImageSize
		}
		return nil
	}

	var err error
	if req.Size, err = normalizeEnumParameter(req.Model, "size", req.Size, capability.Sizes); err != nil {
		return err
	}
	if req.AspectRatio, err = normalizeEnumParameter(req.Model, "aspect_ratio", req.AspectRatio, capability.AspectRatios); err != nil {
		return err
	}
	if req.Resolution, err = normalizeEnumParameter(req.Model, "resolution", req.Resolution, capability.Resolutions); err != nil {
		return err
	}
	if req.Quality, err = normalizeEnumParameter(req.Model, "quality", req.Quality, capability.Quality); err != nil {
		return err
	}
	if req.OutputFormat, err = normalizeEnumParameter(req.Model, "output_format", req.OutputFormat, capability.OutputFormats); err != nil {
		return err
	}
	if req.Background, err = normalizeEnumParameter(req.Model, "background", req.Background, capability.Background); err != nil {
		return err
	}

	if len(req.ReferenceImages) == 0 {
		if strings.TrimSpace(req.InputFidelity) != "" {
			return invalidImageParameter(req.Model, "input_fidelity", req.InputFidelity, "reference images are required")
		}
		req.InputFidelity = ""
	} else if req.InputFidelity, err = normalizeEnumParameter(req.Model, "input_fidelity", req.InputFidelity, capability.InputFidelity); err != nil {
		return err
	}

	if req.OutputCompression != nil {
		if capability.OutputCompression == nil {
			return invalidImageParameter(req.Model, "output_compression", fmt.Sprint(*req.OutputCompression), "unsupported")
		}
		if req.OutputFormat != "jpeg" && req.OutputFormat != "webp" {
			return invalidImageParameter(req.Model, "output_compression", fmt.Sprint(*req.OutputCompression), "output_format must be jpeg or webp")
		}
		if *req.OutputCompression < capability.OutputCompression.Min || *req.OutputCompression > capability.OutputCompression.Max {
			return invalidImageParameter(req.Model, "output_compression", fmt.Sprint(*req.OutputCompression), fmt.Sprintf("range %d..%d", capability.OutputCompression.Min, capability.OutputCompression.Max))
		}
	} else if capability.OutputCompression != nil && (req.OutputFormat == "jpeg" || req.OutputFormat == "webp") {
		value := capability.OutputCompression.Default
		req.OutputCompression = &value
	}
	return nil
}

func normalizeEnumParameter(modelName, parameter, value string, capability *EnumCapability) (string, error) {
	value = strings.TrimSpace(value)
	if capability == nil {
		if value != "" {
			return "", invalidImageParameter(modelName, parameter, value, "unsupported")
		}
		return "", nil
	}
	if value == "" {
		return capability.Default, nil
	}
	if !slices.Contains(capability.Values, value) {
		return "", invalidImageParameter(modelName, parameter, value, "accepted values: "+strings.Join(capability.Values, ", "))
	}
	return value, nil
}

func firstAdvancedImageParameter(req GenerateRequest) (string, string) {
	parameters := []struct{ name, value string }{
		{"quality", req.Quality},
		{"output_format", req.OutputFormat},
		{"background", req.Background},
		{"input_fidelity", req.InputFidelity},
		{"aspect_ratio", req.AspectRatio},
		{"resolution", req.Resolution},
	}
	for _, parameter := range parameters {
		if value := strings.TrimSpace(parameter.value); value != "" {
			return parameter.name, value
		}
	}
	if req.OutputCompression != nil {
		return "output_compression", fmt.Sprint(*req.OutputCompression)
	}
	return "", ""
}

func invalidImageParameter(modelName, parameter, value, constraint string) error {
	return fmt.Errorf("%w: model %s parameter %s value %q (%s)", ErrInvalidImageParameter, modelName, parameter, value, constraint)
}

func validateOutputCount(modelName string, count int) (int, error) {
	if count == 0 {
		count = 1
	}
	limit := imageModelCapability(modelName).MaxOutputImages
	if count < 1 || count > limit {
		return 0, fmt.Errorf("%w: model %s supports between 1 and %d output images", ErrInvalidOutputCount, modelName, limit)
	}
	return count, nil
}

func validateReferenceImageCount(modelName string, references []ReferenceImage) error {
	limit := imageModelCapability(modelName).MaxReferenceImages
	if len(references) <= limit {
		return nil
	}
	return fmt.Errorf("%w: model %s supports at most %d reference images", ErrTooManyReferenceImages, modelName, limit)
}
