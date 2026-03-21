package v1alpha1

// Register registers all walheim/v1alpha1 resource kinds.
// Called explicitly from main; no init() side-effects.
func Register() {
	registerNamespace()
	registerApp()
	registerSecret()
	registerConfigMap()
	registerDaemonSet()
	registerJob()
}
