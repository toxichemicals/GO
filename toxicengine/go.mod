module github.com/toxichemicals/GO/toxicengine

go 1.19 // Updated to Go 1.19

require (
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20250301202403-da16c1255728
	// Changed to require the root mathgl module
	github.com/go-gl/mathgl v1.2.0

	// This module now requires your local 'core' sub-module
	github.com/toxichemicals/GO/toxicengine/core v0.0.0-20250524000000-000000000000-incompatible
)

// This 'replace' directive is CRUCIAL for local development of nested modules.
// It tells Go to use the local path for the 'core' module instead of trying to download it from GitHub.
replace github.com/toxichemicals/GO/toxicengine/core => ./core
