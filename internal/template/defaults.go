package template

// NewDefaultRegistry creates a registry with all built-in templates.
// Templates are compiled into the binary (not external files).
func NewDefaultRegistry() *Registry {
	r := NewRegistry()

	// Register built-in templates
	// Errors are ignored as template names are guaranteed unique
	_ = r.Register(NewBugfixTemplate())
	_ = r.Register(NewFeatureTemplate())
	_ = r.Register(NewCommitTemplate())
	_ = r.Register(NewTaskTemplate())

	return r
}
