module github.com/toxichemicals/GO/toxicengine

go 1.19

replace github.com/toxichemicals/GO/toxicengine/core => ./core

require (
	github.com/go-gl/mathgl v1.2.0
	github.com/toxichemicals/GO/toxicengine/core v0.0.0-00010101000000-000000000000
)

require (
	github.com/go-gl/gl v0.0.0-20231021071112-07e5d0ea2e71 // indirect
	github.com/veandco/go-sdl2 v0.4.40 // indirect
)
