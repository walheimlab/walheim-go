package v1alpha1

// Register registers all walheimlab.github.io/v1alpha1 resource kinds.
// Called explicitly from main; no init() side-effects.
func Register() {
	registerNamespace()
	registerApp()
}
