package web

import "github.com/danielgtaylor/huma/v2"

// Plugin is a self-contained group of API routes mounted under a path prefix.
// Plugins form a hierarchy: a plugin's Register receives an API already scoped
// to its prefix, and it may mount child plugins to nest routes further.
type Plugin interface {
	// Prefix is the path segment this plugin is mounted under, e.g. "/karts".
	// Return "" to register directly on the parent without a new segment.
	Prefix() string
	// Register adds operations (and optionally child plugins via Mount) to the
	// given API, which is already prefixed with everything above this plugin.
	Register(api huma.API)
}

// Mount attaches plugins to parent, giving each its own prefixed group so their
// routes compose into a tree. Call it again inside a plugin's Register to nest
// children.
func Mount(parent huma.API, plugins ...Plugin) {
	for _, p := range plugins {
		api := parent
		if prefix := p.Prefix(); prefix != "" {
			api = huma.NewGroup(parent, prefix)
		}
		p.Register(api)
	}
}
