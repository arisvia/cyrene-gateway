package provider

// ModelRef is a single model entry (id + display name) from an upstream provider.
type ModelRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
