module github.com/toxichemicals/GO/toxicengine

go 1.19

require (
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20250301202403-da16c1255728 // Example version, go mod tidy will fix
	github.com/go-gl/mathgl/mgl32 v1.0.0 // Example version, go mod tidy will fix

	// This module now requires your local 'core' sub-module
	// The specific version doesn't matter much for local 'replace'
	github.com/toxichemicals/GO/toxicengine/core v0.0.0-20250524000000-000000000000-incompatible
)

// This 'replace' directive is CRUCIAL for local development of nested modules.
// It tells Go to use the local path for the 'core' module instead of trying to download it from GitHub.
replace github.com/toxichemicals/GO/toxicengine/core => ./core
