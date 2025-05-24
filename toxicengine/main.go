package main

import (
	"log"

	"github.com/go-gl/mathgl/mgl32" // Still needed for math operations

	"mygameengine.com/engine"      // Import the 'engine' package (your main engine runner)
	"mygameengine.com/engine/core" // Import core to use its types like core.Core in Draw method
)

// Constants for window dimensions
const (
	screenWidth  = 800
	screenHeight = 600
	windowTitle  = "Go OpenGL Rotating Cube Engine"
)

// CubeApp implements the engine.Application interface for our rotating cube.
type CubeApp struct {
	// Rotation state
	totalRotationX float32
	totalRotationY float32
}

// NewCubeApp creates a new instance of our CubeApp.
func NewCubeApp() *CubeApp {
	return &CubeApp{}
}

// Init is called by the engine after OpenGL is ready.
// In this new structure, the VAO/VBO/EBO for the cube
// are now managed directly by the 'core' package.
// So, app.Init() doesn't need to do OpenGL setup for the cube.
func (app *CubeApp) Init() error {
	log.Println("CubeApp initialized.")
	return nil
}

// Update game logic for the cube.
func (app *CubeApp) Update(deltaTime float32) {
	// Update rotation based on delta time for smooth, frame-rate independent rotation
	app.totalRotationY += deltaTime * mgl32.DegToRad(50.0) // Rotate around Y-axis at 50 degrees per second
	app.totalRotationX += deltaTime * mgl32.DegToRad(25.0) // Rotate around X-axis at 25 degrees per second
}

// Draw renders the cube using the Core's high-level DrawCube function.
func (app *CubeApp) Draw(core *core.Core) {
	// --- Update Model Matrix (Rotation) ---
	// Create rotation matrices from accumulated angles
	model := mgl32.Ident4()
	model = model.Mul4(mgl32.HomogRotate3DY(app.totalRotationY)) // Rotate around Y-axis
	model = model.Mul4(mgl32.HomogRotate3DX(app.totalRotationX)) // Rotate around X-axis

	// Pass the calculated model matrix to the Core for drawing
	core.DrawCube(model)
}

// Shutdown cleans up cube-specific resources (none directly managed by app in this setup).
func (app *CubeApp) Shutdown() {
	log.Println("CubeApp shutting down.")
	// No specific OpenGL resources to clean up here, as they are managed by the Core.
}

func main() {
	// Create our specific game application (the rotating cube)
	cubeApp := NewCubeApp()

	// Create a new engine instance, passing our application to it
	eng := engine.New(screenWidth, screenHeight, windowTitle, cubeApp)

	// Initialize the engine (which also initializes the core and the app)
	if err := eng.Init(); err != nil {
		log.Fatalf("Engine initialization failed: %v", err)
	}
	defer eng.Shutdown() // Ensure the engine is properly shut down (which also shuts down the app)

	// Run the main engine loop
	eng.Run()
}
