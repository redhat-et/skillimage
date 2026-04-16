package skillcard

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/redhat-et/skillimage/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

var compiledSchema *jsonschema.Schema

func init() {
	// Load and compile the JSON Schema once at package init time
	var schemaDoc any
	if err := json.Unmarshal(schemas.SkillCardV1, &schemaDoc); err != nil {
		panic(fmt.Sprintf("failed to unmarshal embedded schema: %v", err))
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("skillcard-v1.json", schemaDoc); err != nil {
		panic(fmt.Sprintf("failed to add schema resource: %v", err))
	}

	var err error
	compiledSchema, err = compiler.Compile("skillcard-v1.json")
	if err != nil {
		panic(fmt.Sprintf("failed to compile schema: %v", err))
	}
}

// ValidationError represents a single validation error with field path and message.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) String() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate validates a SkillCard against the JSON Schema and additional semantic rules.
// Returns a slice of ValidationError for any validation failures.
func Validate(sc *SkillCard) ([]ValidationError, error) {
	var errors []ValidationError

	// Marshal SkillCard to YAML, then unmarshal to map[string]any to preserve kebab-case keys
	yamlBytes, err := yaml.Marshal(sc)
	if err != nil {
		return nil, fmt.Errorf("marshaling skillcard for validation: %w", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return nil, fmt.Errorf("unmarshaling skillcard for validation: %w", err)
	}

	// Validate against JSON Schema
	if err := compiledSchema.Validate(doc); err != nil {
		// Extract validation errors
		if validationErr, ok := err.(*jsonschema.ValidationError); ok {
			errors = append(errors, collectValidationErrors(validationErr)...)
		} else {
			return nil, fmt.Errorf("unexpected validation error type: %w", err)
		}
	}

	// Additional semver validation
	if sc.Metadata.Version != "" {
		if _, err := semver.StrictNewVersion(sc.Metadata.Version); err != nil {
			errors = append(errors, ValidationError{
				Field:   "metadata.version",
				Message: fmt.Sprintf("invalid semantic version: %v", err),
			})
		}
	}

	return errors, nil
}

// collectValidationErrors recursively collects leaf validation errors from the jsonschema ValidationError tree.
func collectValidationErrors(ve *jsonschema.ValidationError) []ValidationError {
	var errors []ValidationError

	// If there are causes, recurse into them
	if len(ve.Causes) > 0 {
		for _, cause := range ve.Causes {
			errors = append(errors, collectValidationErrors(cause)...)
		}
		return errors
	}

	// This is a leaf error - extract field path and message
	fieldPath := strings.Join(ve.InstanceLocation, ".")
	if fieldPath == "" {
		fieldPath = "(root)"
	}

	message := ve.Error()
	// Clean up the error message to remove the instance location prefix
	// The error format is typically: "instanceLocation: message"
	if idx := strings.Index(message, ": "); idx != -1 {
		message = message[idx+2:]
	}

	errors = append(errors, ValidationError{
		Field:   fieldPath,
		Message: message,
	})

	return errors
}
