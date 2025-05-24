package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"
	"unsafe" // For gl.PtrOffset

	"github.com/go-gl/gl/v4.6-core/gl" // Still using this for OpenGL functions
	"github.com/go-gl/glfw/v3.3/glfw" // NEW: GLFW for windowing
	"github.com/go-gl/mathgl/mgl32"
)

// Constants for window dimensions
const (
	screenWidth  = 800
	screenHeight = 600
	windowTitle  = "Go OpenGL Rotating Cube Engine (GLFW)"
)

// AppCore struct encapsulates the low-level graphics and windowing components.
type AppCore struct {
	window *glfw.Window // Changed from *sdl.Window to *glfw.Window

	// OpenGL program and buffers for the cube
	program      uint32
	vao          uint32
	vbo          uint32
	ebo          uint32
	indicesCount int32

	// Uniform locations
	modelUniform      int32
	viewUniform       int32
	projectionUniform int32

	// Window dimensions
	width, height int
	title         string

	// Internal state for main loop
	running bool

	// Cube data
	vertices []float32
	indices  []uint32
}

// Global instance of AppCore
var app *AppCore

// initApp initializes GLFW, OpenGL context, and prepares the core rendering data.
func initApp() error {
	runtime.LockOSThread() // GLFW requires this
	// defer runtime.UnlockOSThread() // This will be handled by the main defer

	app = &AppCore{
		width:   screenWidth,
		height:  screenHeight,
		title:   windowTitle,
		running: true, // Start as running
	}

	// Initialize GLFW
	if err := glfw.Init(); err != nil {
		return fmt.Errorf("failed to initialize GLFW: %w", err)
	}

	// Set GLFW window hints for OpenGL context
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 6) // Matching v4.6-core/gl
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True) // Required for macOS

	window, err := glfw.CreateWindow(app.width, app.height, app.title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("failed to create GLFW window: %w", err)
	}
	app.window = window
	app.window.MakeContextCurrent() // Make the OpenGL context current

	// Set a callback for window resizing
	app.window.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		app.width = width
		app.height = height
		gl.Viewport(0, 0, int32(width), int32(height))
		projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(app.width)/float32(app.height), 0.1, 100.0)
		gl.UniformMatrix4fv(app.projectionUniform, 1, false, &projection[0])
	})

	// Initialize GL
	if err := gl.Init(); err != nil {
		glfw.Terminate()
		return fmt.Errorf("failed to initialize OpenGL: %w", err)
	}

	gl.Enable(gl.DEPTH_TEST)
	gl.Viewport(0, 0, int32(app.width), int32(app.height))

	// --- Cube Data (unchanged) ---
	app.vertices = []float32{
		// Front face (Red)
		-0.5, -0.5, 0.5, 1.0, 0.0, 0.0,
		0.5, -0.5, 0.5, 1.0, 0.0, 0.0,
		0.5, 0.5, 0.5, 1.0, 0.0, 0.0,
		-0.5, 0.5, 0.5, 1.0, 0.0, 0.0,

		// Back face (Green)
		-0.5, -0.5, -0.5, 0.0, 1.0, 0.0,
		0.5, -0.5, -0.5, 0.0, 1.0, 0.0,
		0.5, 0.5, -0.5, 0.0, 1.0, 0.0,
		-0.5, 0.5, -0.5, 0.0, 1.0, 0.0,

		// Right face (Blue)
		0.5, -0.5, 0.5, 0.0, 0.0, 1.0,
		0.5, -0.5, -0.5, 0.0, 0.0, 1.0,
		0.5, 0.5, -0.5, 0.0, 0.0, 1.0,
		0.5, 0.5, 0.5, 0.0, 0.0, 1.0,

		// Left face (Yellow)
		-0.5, -0.5, 0.5, 1.0, 1.0, 0.0,
		-0.5, -0.5, -0.5, 1.0, 1.0, 0.0,
		-0.5, 0.5, -0.5, 1.0, 1.0, 0.0,
		-0.5, 0.5, 0.5, 1.0, 1.0, 0.0,

		// Top face (Cyan)
		-0.5, 0.5, 0.5, 0.0, 1.0, 1.0,
		0.5, 0.5, 0.5, 0.0, 1.0, 1.0,
		0.5, 0.5, -0.5, 0.0, 1.0, 1.0,
		-0.5, 0.5, -0.5, 0.0, 1.0, 1.0,

		// Bottom face (Magenta)
		-0.5, -0.5, 0.5, 1.0, 0.0, 1.0,
		0.5, -0.5, 0.5, 1.0, 0.0, 1.0,
		0.5, -0.5, -0.5, 1.0, 0.0, 1.0,
		-0.5, -0.5, -0.5, 1.0, 0.0, 1.0,
	}
	app.indices = []uint32{
		// Front
		0, 1, 2,
		2, 3, 0,

		// Back
		4, 5, 6,
		6, 7, 4,

		// Right
		8, 9, 10,
		10, 11, 8,

		// Left
		12, 13, 14,
		14, 15, 12,

		// Top
		16, 17, 18,
		18, 19, 16,

		// Bottom
		20, 21, 22,
		22, 23, 20,
	}
	app.indicesCount = int32(len(app.indices))

	// --- Shader Program Setup (unchanged) ---
	vertexShaderSource := `
		#version 410 core
		layout (location = 0) in vec3 aPos;
		layout (location = 1) in vec3 aColor;

		out vec3 ourColor;

		uniform mat4 model;
		uniform mat4 view;
		uniform mat4 projection;

		void main() {
			gl_Position = projection * view * model * vec4(aPos, 1.0);
			ourColor = aColor;
		}
	` + "\x00"

	fragmentShaderSource := `
		#version 410 core
		in vec3 ourColor;
		out vec4 FragColor;

		void main() {
			FragColor = vec4(ourColor, 1.0);
		}
	` + "\x00"

	program, err := compileShader(vertexShaderSource, fragmentShaderSource)
	if err != nil {
		app.window.Destroy()
		glfw.Terminate()
		return fmt.Errorf("failed to compile shaders: %w", err)
	}
	gl.UseProgram(program)
	app.program = program

	// Get uniform locations
	app.modelUniform = gl.GetUniformLocation(app.program, gl.Str("model\x00"))
	app.viewUniform = gl.GetUniformLocation(app.program, gl.Str("view\x00"))
	app.projectionUniform = gl.GetUniformLocation(app.program, gl.Str("projection\x00"))

	// --- VBO, VAO, EBO Setup for the cube (unchanged) ---
	gl.GenVertexArrays(1, &app.vao)
	gl.BindVertexArray(app.vao)

	gl.GenBuffers(1, &app.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, app.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(app.vertices)*4, gl.Ptr(app.vertices), gl.STATIC_DRAW)

	gl.GenBuffers(1, &app.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, app.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(app.indices)*4, gl.Ptr(app.indices), gl.STATIC_DRAW)

	// Position attribute (layout location 0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 6*4, gl.Ptr(nil))
	gl.EnableVertexAttribArray(0)

	// Color attribute (layout location 1)
	gl.VertexAttribPointer(1, 3, gl.FLOAT, false, 6*4, gl.PtrOffset(3*4))
	gl.EnableVertexAttribArray(1)

	gl.BindVertexArray(0) // Unbind VAO

	// --- Camera (View Matrix) Setup (unchanged) ---
	cameraPos := mgl32.Vec3{0, 0, 3}
	cameraFront := mgl32.Vec3{0, 0, -1}
	cameraUp := mgl32.Vec3{0, 1, 0}
	view := mgl32.LookAtV(cameraPos, cameraPos.Add(cameraFront), cameraUp)
	gl.UniformMatrix4fv(app.viewUniform, 1, false, &view[0])

	// --- Projection Matrix Setup (Perspective) (unchanged) ---
	projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(app.width)/float32(app.height), 0.1, 100.0)
	gl.UniformMatrix4fv(app.projectionUniform, 1, false, &projection[0])

	return nil
}

// shouldClose returns true if the window should close.
func shouldClose() bool {
	return app.window.ShouldClose() || !app.running
}

// pollEvents processes window events (GLFW)
func pollEvents() {
	glfw.PollEvents() // GLFW's way of processing events
	// You can set key/mouse callbacks here if needed, for simplicity we rely on ShouldClose
	if app.window.GetKey(glfw.KeyEscape) == glfw.Press {
		app.running = false // Set running to false if escape is pressed
	}
}

// clearFrame clears the color and depth buffers.
func clearFrame() {
	gl.ClearColor(0.2, 0.3, 0.3, 1.0) // Dark teal background
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
}

// swapBuffers swaps the front and back buffers to display the rendered frame.
func swapBuffers() {
	app.window.SwapBuffers() // GLFW equivalent of glfw.SwapBuffers
}

// drawCube draws the predefined cube with the given model matrix.
func drawCube(modelMatrix mgl32.Mat4) {
	gl.UniformMatrix4fv(app.modelUniform, 1, false, &modelMatrix[0])

	gl.BindVertexArray(app.vao)
	gl.DrawElements(gl.TRIANGLES, app.indicesCount, gl.UNSIGNED_INT, unsafe.Pointer(uintptr(0)))
	gl.BindVertexArray(0)
}

// shutdownApp cleans up core OpenGL and GLFW resources.
func shutdownApp() {
	gl.DeleteVertexArrays(1, &app.vao)
	gl.DeleteBuffers(1, &app.vbo)
	gl.DeleteBuffers(1, &app.ebo)
	gl.DeleteProgram(app.program)

	if app.window != nil {
		app.window.Destroy()
	}
	glfw.Terminate() // Terminate GLFW
}

// --- Helper functions for shader compilation (unchanged) ---

// compileShader compiles vertex and fragment shaders into an OpenGL program.
func compileShader(vertexShaderSource, fragmentShaderSource string) (uint32, error) {
	vertexShader := gl.CreateShader(gl.VERTEX_SHADER)
	glShaderSource(vertexShader, vertexShaderSource)
	gl.CompileShader(vertexShader)
	if err := checkShaderCompileStatus(vertexShader, "vertex"); err != nil {
		return 0, err
	}

	fragmentShader := gl.CreateShader(gl.FRAGMENT_SHADER)
	glShaderSource(fragmentShader, fragmentShaderSource)
	gl.CompileShader(fragmentShader)
	if err := checkShaderCompileStatus(fragmentShader, "fragment"); err != nil {
		return 0, err
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)
	if err := checkProgramLinkStatus(program); err != nil {
		return 0, err
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	return program, nil
}

// glShaderSource is a helper to correctly pass GLSL source to OpenGL
func glShaderSource(shader uint32, source string) {
	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
}

// checkShaderCompileStatus checks if a shader compiled successfully.
func checkShaderCompileStatus(shader uint32, shaderType string) error {
	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))
		return fmt.Errorf("failed to compile %s shader:\n%v", shaderType, log)
	}
	return nil
}

// checkProgramLinkStatus checks if a shader program linked successfully.
func checkProgramLinkStatus(program uint32) error {
	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))
		return fmt.Errorf("failed to link program:\n%v", log)
	}
	return nil
}

func main() {
	runtime.LockOSThread() // Required for GLFW
	defer runtime.UnlockOSThread()
	defer shutdownApp() // Ensure shutdown is called

	if err := initApp(); err != nil {
		log.Fatalf("Application initialization failed: %v", err)
	}

	log.Println("Engine (main.go) initialized. Starting main loop...")

	// --- Game State Variables (for the rotating cube) ---
	var totalRotationX float32
	var totalRotationY float32
	lastFrameTime := time.Now() // To calculate deltaTime for smooth, frame-rate independent motion

	// --- Main Game Loop ---
	for !shouldClose() { // Loop as long as the window is not requested to close
		// Calculate delta time
		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(lastFrameTime).Seconds())
		lastFrameTime = currentTime

		// 1. Event Processing: Poll for and process window events (handled by GLFW)
		pollEvents()

		// 2. Clear Frame: Clear the screen's color and depth buffers
		clearFrame()

		// 3. Update Game Logic: Calculate new rotation values based on delta time
		totalRotationY += deltaTime * mgl32.DegToRad(50.0) // Rotate around Y-axis at 50 degrees per second
		totalRotationX += deltaTime * mgl32.DegToRad(25.0) // Rotate around X-axis at 25 degrees per second

		// 4. Render (Draw Cube):
		//    a. Calculate the model matrix for the cube's current rotation
		model := mgl32.Ident4() // Start with an identity matrix
		model = model.Mul4(mgl32.HomogRotate3DY(totalRotationY)) // Apply Y-axis rotation
		model = model.Mul4(mgl32.HomogRotate3DX(totalRotationX)) // Apply X-axis rotation

		//    b. Pass the calculated model matrix to the core library for drawing
		drawCube(model)

		// 5. Swap Buffers: Display the rendered frame on the screen
		swapBuffers()
	}

	log.Println("Engine (main.go) shutting down.")
}
