package adapters

import (
	"path/filepath"
	"regexp"
	"strings"
)

var environmentKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type Registry struct{ entries map[string]Descriptor }

func NewRegistry(descriptors ...Descriptor) (*Registry, error) {
	registry := &Registry{entries: make(map[string]Descriptor, len(descriptors))}
	for _, descriptor := range descriptors {
		if err := validateDescriptor(descriptor); err != nil {
			return nil, err
		}
		if _, exists := registry.entries[descriptor.AdapterID]; exists {
			return nil, runtimeError("duplicate_adapter")
		}
		copyDescriptor := descriptor
		copyDescriptor.Arguments = append([]string(nil), descriptor.Arguments...)
		copyDescriptor.Environment = cloneStrings(descriptor.Environment)
		registry.entries[descriptor.AdapterID] = copyDescriptor
	}
	return registry, nil
}

func validateDescriptor(descriptor Descriptor) error {
	if !identifierPattern.MatchString(descriptor.AdapterID) ||
		!filepath.IsAbs(descriptor.Executable) || strings.ContainsRune(descriptor.Executable, '\x00') ||
		(descriptor.WorkingDirectory != "" && !filepath.IsAbs(descriptor.WorkingDirectory)) {
		return runtimeError("invalid_descriptor")
	}
	for _, argument := range descriptor.Arguments {
		if strings.ContainsRune(argument, '\x00') {
			return runtimeError("invalid_descriptor")
		}
	}
	for key, value := range descriptor.Environment {
		if !environmentKeyPattern.MatchString(key) || strings.ContainsRune(value, '\x00') {
			return runtimeError("invalid_descriptor")
		}
	}
	return nil
}

func (registry *Registry) Lookup(adapterID string) (Descriptor, error) {
	descriptor, exists := registry.entries[adapterID]
	if !exists {
		return Descriptor{}, runtimeError("adapter_not_registered")
	}
	descriptor.Arguments = append([]string(nil), descriptor.Arguments...)
	descriptor.Environment = cloneStrings(descriptor.Environment)
	return descriptor, nil
}

func (registry *Registry) Count() int { return len(registry.entries) }

func cloneStrings(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
