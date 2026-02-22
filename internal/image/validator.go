package image

import (
	"fmt"
	"strings"
)

// Validator checks images against approved/blocked lists.
type Validator struct {
	approved []string
	blocked  []string
}

// NewValidator creates an image validator.
func NewValidator(approved, blocked []string) *Validator {
	return &Validator{
		approved: approved,
		blocked:  blocked,
	}
}

// Validate checks if an image is allowed.
func (v *Validator) Validate(image string) error {
	// If no lists configured, allow all
	if len(v.approved) == 0 && len(v.blocked) == 0 {
		return nil
	}

	// Check blocked first
	for _, pattern := range v.blocked {
		if v.matches(image, pattern) {
			return fmt.Errorf("image %s is blocked (matches %s)", image, pattern)
		}
	}

	// If approved list exists, image must match one
	if len(v.approved) > 0 {
		for _, pattern := range v.approved {
			if v.matches(image, pattern) {
				return nil
			}
		}
		return fmt.Errorf("image %s is not in approved list", image)
	}

	return nil
}

func (v *Validator) matches(image, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(image, prefix)
	}
	return image == pattern
}
