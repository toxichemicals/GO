module github.com/toxichemicals/GO/toxicengine

go 1.19 // Your Go version

require (
	// Removed github.com/go-gl/glfw/v3.3/glfw
	github.com/go-gl/mathgl v1.2.0
	github.com/veandco/go-sdl2/sdl v0.0.0-20240327072048-cd01b59c7a72 // Added for SDL2

	// This module now requires your local 'core' sub-module
	github.com/toxichemicals/GO/toxicengine/core v0.0.0-20250524000000-000000000000-incompatible
)

// This 'replace' directive is CRUCIAL for local development of nested modules.
// It tells Go to use the local path for the 'core' module instead of trying to download it from GitHub.
replace github.com/toxichemicals/GO/toxicengine/core => ./core
