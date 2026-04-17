package controller

import "net/http"

// Registrable is implemented by all controllers to register their routes.
type Registrable interface {
	RegisterRoutes(mux *http.ServeMux)
}

// RegisterAll mounts all controllers' routes onto the mux.
func RegisterAll(mux *http.ServeMux, controllers ...Registrable) {
	for _, c := range controllers {
		c.RegisterRoutes(mux)
	}
}
