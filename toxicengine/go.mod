module github.com/toxichemicals/GO/toxicengine

go 1.19

require (
	github.com/go-gl/mathgl v1.2.0
	github.com/veandco/go-sdl2/sdl // REMOVED THE SPECIFIC PSEUDO-VERSION

	// This module now requires your local 'core' sub-module
	github.com/toxichemicals/GO/toxicengine/core v0.0.0-20250524000000-000000000000-incompatible
)

replace github.com/toxichemicals/GO/toxicengine/core => ./core
