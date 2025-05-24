package main

import (
	"log"
	"time"

	"github.com/go-gl/mathgl/mgl32" // For math operations like mgl32.DegToRad

	// Import your 'core' sub-module directly using its full module path
	"github.com/toxichemicals/GO/toxicengine/core"
)

// Constants for window dimensions
const (
	screenWidth  = 800
	screenHeight = 600
	windowTitle  = "Go OpenGL Rotating Cube Engine"
)

func main() {
	// Create an instance of your low-level core graphics library
	// This replaces the "engine.New" call from our previous, more abstract design.
	coreLib := core.NewCore(screenWidth, screenHeight, windowTitle)

	// Initialize the core graphics library (which sets up GLFW, OpenGL, shaders, etc.)
	if err := coreLib.Init(); err != nil {
		log.Fatalf("Core library initialization failed: %v", err)
	}
	// Ensure the core library is properly shut down when main exits, cleaning up resources.
	defer coreLib.Shutdown()

	log.Println("Engine (main.go) initialized. Starting main loop...")

	// --- Game State Variables (for the rotating cube) ---
	var totalRotationX float32
	var totalRotationY float32
	lastFrameTime := time.Now() // To calculate deltaTime for smooth, frame-rate independent motion

	// --- Main Game Loop ---
	for !coreLib.ShouldClose() { // Loop as long as the window is not requested to close
		// Calculate delta time
		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(lastFrameTime).Seconds())
		lastFrameTime = currentTime

		// 1. Event Processing: Poll for and process window events (like keyboard, mouse, window close)
		coreLib.PollEvents()

		// 2. Clear Frame: Clear the screen's color and depth buffers
		coreLib.ClearFrame()

		// 3. Update Game Logic: Calculate new rotation values based on delta time
		totalRotationY += deltaTime * mgl32.DegToRad(50.0) // Rotate around Y-axis at 50 degrees per second
		totalRotationX += deltaTime * mgl32.DegToRad(25.0) // Rotate around X-axis at 25 degrees per second

		// 4. Render (Draw Cube):
		//    a. Calculate the model matrix for the cube's current rotation
		model := mgl32.Ident4() // Start with an identity matrix
		model = model.Mul4(mgl32.HomogRotate3DY(totalRotationY)) // Apply Y-axis rotation
		model = model.Mul4(mgl32.HomogRotate3DX(totalRotationX)) // Apply X-axis rotation

		//    b. Pass the calculated model matrix to the core library for drawing
		coreLib.DrawCube(model)

		// 5. Swap Buffers: Display the rendered frame on the screen
		coreLib.SwapBuffers()
	}

	log.Println("Engine (main.go) shutting down.")
}
