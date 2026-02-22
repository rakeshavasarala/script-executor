package image

import (
	"context"
	"fmt"
)

// Resolver resolves image references to full image specs.
type Resolver struct {
	catalog  *Catalog
	defaults ResolverDefaults
}

// ResolverDefaults holds default image settings.
type ResolverDefaults struct {
	Image          string
	PullSecret     string
	PullPolicy     string
}

// NewResolver creates an image resolver.
func NewResolver(catalog *Catalog, defaults ResolverDefaults) *Resolver {
	if defaults.PullPolicy == "" {
		defaults.PullPolicy = "IfNotPresent"
	}
	return &Resolver{
		catalog:  catalog,
		defaults: defaults,
	}
}

// Resolve resolves an image or image_ref to a full ResolvedImage.
func (r *Resolver) Resolve(ctx context.Context, image, imageRef, imagePullPolicy, imagePullSecret string) (*ResolvedImage, error) {
	var resolved ResolvedImage

	if image != "" {
		resolved.Image = image
	} else if imageRef != "" {
		if err := r.catalog.Load(ctx); err != nil {
			return nil, fmt.Errorf("load image catalog: %w", err)
		}
		entry, ok := r.catalog.Get(imageRef)
		if !ok {
			return nil, fmt.Errorf("image_ref %q not found in catalog", imageRef)
		}
		resolved.Image = entry.Image
		if entry.PullSecret != "" {
			resolved.PullSecret = entry.PullSecret
		}
	} else {
		resolved.Image = r.defaults.Image
	}

	if resolved.PullSecret == "" {
		resolved.PullSecret = imagePullSecret
		if resolved.PullSecret == "" {
			resolved.PullSecret = r.defaults.PullSecret
		}
	}

	resolved.PullPolicy = imagePullPolicy
	if resolved.PullPolicy == "" {
		resolved.PullPolicy = r.defaults.PullPolicy
	}

	return &resolved, nil
}
