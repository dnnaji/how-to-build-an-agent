package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PathAccess represents the type of access requested for a path.
type PathAccess int

const (
	AccessReadFile PathAccess = iota
	AccessWriteFile
	AccessListDir
)

// PathSandbox enforces filesystem access within a configured root.
type PathSandbox struct {
	Root string // Resolved absolute path to the root
}

// NewPathSandbox creates a new sandbox with the given root.
// It resolves the root to an absolute path and evaluates symlinks.
func NewPathSandbox(root string) (*PathSandbox, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root: %w", err)
	}

	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate root symlinks: %w", err)
	}

	return &PathSandbox{
		Root: rootReal,
	}, nil
}

// Resolve validates and resolves a user-provided path within the sandbox.
// Returns the absolute real path or an error with suggestions.
func (s *PathSandbox) Resolve(userPath string, access PathAccess) (string, error) {
	// 1. Reject empty/whitespace-only paths
	if strings.TrimSpace(userPath) == "" {
		return "", &SandboxError{
			Code:    "invalid_argument",
			Message: "path cannot be empty",
		}
	}

	// 2. Normalize and validate
	clean := filepath.Clean(userPath)

	// Optionally reject absolute paths; for now, we allow them but still validate.
	var candidate string
	if filepath.IsAbs(clean) {
		candidate = clean
	} else {
		candidate = filepath.Join(s.Root, clean)
	}

	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", &SandboxError{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("failed to resolve path: %v", err),
		}
	}

	// 4. Symlink protection
	var candidateReal string
	switch access {
	case AccessReadFile, AccessListDir:
		// For read/list: must evaluate symlinks successfully
		real, err := filepath.EvalSymlinks(candidateAbs)
		if err != nil {
			// Suggest alternatives
			suggestions := s.suggestFiles(candidateAbs)
			return "", &SandboxError{
				Code:        "not_found",
				Message:     fmt.Sprintf("path not found: %s", userPath),
				Suggestions: suggestions,
			}
		}
		candidateReal = real

	case AccessWriteFile:
		// For write: if it exists, use evaluated path; otherwise eval parent.
		real, err := filepath.EvalSymlinks(candidateAbs)
		if err == nil {
			candidateReal = real
		} else {
			// File doesn't exist; eval parent to detect symlinked parent escapes
			parentAbs := filepath.Dir(candidateAbs)
			parentReal, err := filepath.EvalSymlinks(parentAbs)
			if err != nil {
				// Parent doesn't exist either
				suggestions := s.suggestFiles(parentAbs)
				return "", &SandboxError{
					Code:        "not_found",
					Message:     fmt.Sprintf("parent directory not found: %s", filepath.Dir(userPath)),
					Suggestions: suggestions,
				}
			}
			candidateReal = filepath.Join(parentReal, filepath.Base(candidateAbs))
		}
	}

	// 5. Root check
	rel, err := filepath.Rel(s.Root, candidateReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", &SandboxError{
			Code:    "permission_denied",
			Message: fmt.Sprintf("path escapes project root: %s", userPath),
			Suggestions: []string{
				"Use a relative path under the project root (e.g., './subdir/file.txt')",
			},
		}
	}

	return candidateReal, nil
}

// suggestFiles returns up to 3 file/dir name suggestions from the parent directory.
func (s *PathSandbox) suggestFiles(path string) []string {
	parentDir := filepath.Dir(path)
	baseName := filepath.Base(path)

	entries, err := filepath.Glob(filepath.Join(parentDir, "*"))
	if err != nil {
		return nil
	}

	// Extract just the names
	var names []string
	for _, entry := range entries {
		name := filepath.Base(entry)
		names = append(names, name)
	}

	// Simple matching: prefix match, then substring, then none
	var matches []string

	// Case-insensitive prefix match
	for _, name := range names {
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(baseName)) {
			matches = append(matches, name)
			if len(matches) >= 3 {
				return formatSuggestions(matches)
			}
		}
	}

	// Substring match
	for _, name := range names {
		if strings.Contains(strings.ToLower(name), strings.ToLower(baseName)) {
			// Skip if already added
			var found bool
			for _, m := range matches {
				if m == name {
					found = true
					break
				}
			}
			if !found {
				matches = append(matches, name)
				if len(matches) >= 3 {
					return formatSuggestions(matches)
				}
			}
		}
	}

	if len(matches) > 0 {
		return formatSuggestions(matches)
	}
	return nil
}

// formatSuggestions wraps suggestions in "Did you mean..." messages.
func formatSuggestions(names []string) []string {
	var suggestions []string
	for i, name := range names {
		if i >= 3 {
			break
		}
		suggestions = append(suggestions, fmt.Sprintf("Did you mean '%s'?", name))
	}
	return suggestions
}

// SandboxError is a structured error for sandbox violations.
type SandboxError struct {
	Code        string   // not_found, invalid_argument, permission_denied, io_error
	Message     string
	Suggestions []string
}

func (e *SandboxError) Error() string {
	return e.Message
}
