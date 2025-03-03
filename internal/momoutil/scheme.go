package momoutil

import "k8s.io/apimachinery/pkg/runtime"

func NewScheme(addToScheme ...func(*runtime.Scheme) error) (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(addToScheme...)
	return scheme, schemeBuilder.AddToScheme(scheme)
}
