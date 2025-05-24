module github.com/toxichemicals/GO/toxicengine/core

go 1.19

require (
	github.com/go-gl/gl/v4.1-core/gl v0.0.0-20231021071112-07e5d0ea2e71 // Example version, go mod tidy will fix
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20250301202403-da16c1255728 // GLFW is also needed by core.go directly
	github.com/go-gl/mathgl/mgl32 v1.0.0 // Mathgl is also needed by core.go directly
)
