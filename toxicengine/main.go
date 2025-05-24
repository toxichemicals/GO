package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"
	"unsafe" // For gl.PtrOffset

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
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
	window *glfw.Window

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
	title         string // Original window title

	// Internal state for main loop
	running bool

	// Cube data
	vertices []float32
	indices  []uint32

	// Game state for animation
	totalRotationX float32
	totalRotationY float32
	lastFrameTime  time.Time

	// FPS Counter state
	fpsFrames      int       // Number of frames rendered in the current second
	fpsLastUpdateTime time.Time // Last time the FPS was updated/displayed

	// VSync Control state
	vsyncEnabled    bool // Current VSync state (true = on, false = off)
	vKeyWasPressed bool // Debounce flag for 'V' key
}

// Global instance of AppCore
var app *AppCore

// initApp initializes GLFW, OpenGL context, and prepares the core rendering data.
// This function will now orchestrate calls to more granular initialization steps.
func initApp() error {
	runtime.LockOSThread() // GLFW requires this

	app = &AppCore{
		width:   screenWidth,
		height:  screenHeight,
		title:   windowTitle, // Store the base title
		running: true,        // Start as running
		vertices: []float32{
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
		},
		indices: []uint32{
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
		},
	}
	app.indicesCount = int32(len(app.indices))

	if err := app.initializeWindow(); err != nil {
		return fmt.Errorf("window initialization failed: %w", err)
	}

	if err := app.initializeOpenGL(); err != nil {
		return fmt.Errorf("OpenGL initialization failed: %w", err)
	}

	if err := app.setupShadersAndUniforms(); err != nil {
		return fmt.Errorf("shader setup failed: %w", err)
	}

	if err := app.setupCubeBuffers(); err != nil {
		return fmt.Errorf("cube buffer setup failed: %w", err)
	}

	app.setupCameraAndProjection()

	app.lastFrameTime = time.Now()      // Initialize lastFrameTime for delta time calculation
	app.fpsLastUpdateTime = time.Now() // Initialize for FPS counter
	app.fpsFrames = 0                   // Initialize frame counter

	return nil
}

// initializeWindow handles GLFW initialization and window creation.
func (a *AppCore) initializeWindow() error {
	if err := glfw.Init(); err != nil {
		return fmt.Errorf("failed to initialize GLFW: %w", err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 6)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(a.width, a.height, a.title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("failed to create GLFW window: %w", err)
	}
	a.window = window
	a.window.MakeContextCurrent()

	// Initialize VSync state to ON (capped)
	a.vsyncEnabled = true
	glfw.SwapInterval(1) // 1 for VSync ON, 0 for VSync OFF
	log.Println("VSync ON (default)")

	a.window.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		a.width = width
		a.height = height
		gl.Viewport(0, 0, int32(width), int32(height))
		// Re-calculate projection matrix on resize
		projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(a.width)/float32(a.height), 0.1, 100.0)
		gl.UniformMatrix4fv(a.projectionUniform, 1, false, &projection[0])
	})

	return nil
}

// initializeOpenGL initializes GL and sets global OpenGL states.
func (a *AppCore) initializeOpenGL() error {
	if err := gl.Init(); err != nil {
		glfw.Terminate()
		return fmt.Errorf("failed to initialize OpenGL: %w", err)
	}

	gl.Enable(gl.DEPTH_TEST)
	gl.Viewport(0, 0, int32(a.width), int32(a.height))
	return nil
}

// setupShadersAndUniforms compiles shaders, links the program, and gets uniform locations.
func (a *AppCore) setupShadersAndUniforms() error {
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
		return fmt.Errorf("failed to compile shaders: %w", err)
	}
	gl.UseProgram(program)
	a.program = program

	a.modelUniform = gl.GetUniformLocation(a.program, gl.Str("model\x00"))
	a.viewUniform = gl.GetUniformLocation(a.program, gl.Str("view\x00"))
	a.projectionUniform = gl.GetUniformLocation(a.program, gl.Str("projection\x00"))

	return nil
}

// setupCubeBuffers configures VAO, VBO, and EBO for the cube data.
func (a *AppCore) setupCubeBuffers() error {
	gl.GenVertexArrays(1, &a.vao)
	gl.BindVertexArray(a.vao)

	gl.GenBuffers(1, &a.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, a.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(a.vertices)*4, gl.Ptr(a.vertices), gl.STATIC_DRAW)

	gl.GenBuffers(1, &a.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, a.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(a.indices)*4, gl.Ptr(a.indices), gl.STATIC_DRAW)

	// Position attribute (layout location 0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 6*4, gl.Ptr(nil))
	gl.EnableVertexAttribArray(0)

	// Color attribute (layout location 1)
	gl.VertexAttribPointer(1, 3, gl.FLOAT, false, 6*4, gl.PtrOffset(3*4))
	gl.EnableVertexAttribArray(1)

	gl.BindVertexArray(0) // Unbind VAO

	return nil
}

// setupCameraAndProjection sets up initial view and projection matrices.
func (a *AppCore) setupCameraAndProjection() {
	cameraPos := mgl32.Vec3{0, 0, 3}
	cameraFront := mgl32.Vec3{0, 0, -1}
	cameraUp := mgl32.Vec3{0, 1, 0}
	view := mgl32.LookAtV(cameraPos, cameraPos.Add(cameraFront), cameraUp)
	gl.UniformMatrix4fv(a.viewUniform, 1, false, &view[0])

	projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(a.width)/float32(a.height), 0.1, 100.0)
	gl.UniformMatrix4fv(a.projectionUniform, 1, false, &projection[0])
}

// processInput handles keyboard/mouse input.
func (a *AppCore) processInput() {
	glfw.PollEvents()

	if a.window.GetKey(glfw.KeyEscape) == glfw.Press {
		a.running = false
	}

	// VSync toggle logic
	currentVState := a.window.GetKey(glfw.KeyV)
	if currentVState == glfw.Press && !a.vKeyWasPressed {
		a.vsyncEnabled = !a.vsyncEnabled
		if a.vsyncEnabled {
			glfw.SwapInterval(1) // Enable VSync
			log.Println("VSync: ON (FPS capped)")
		} else {
			glfw.SwapInterval(0) // Disable VSync
			log.Println("VSync: OFF (FPS uncapped)")
		}
	}
	a.vKeyWasPressed = (currentVState == glfw.Press)
}

// updateScene updates the game state (e.g., cube rotation).
func (a *AppCore) updateScene(deltaTime float32) {
	a.totalRotationY += deltaTime * mgl32.DegToRad(50.0)
	a.totalRotationX += deltaTime * mgl32.DegToRad(25.0)
}

// renderScene clears buffers, draws the cube, and swaps buffers.
func (a *AppCore) renderScene() {
	gl.ClearColor(0.2, 0.3, 0.3, 1.0)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	// Calculate the model matrix for the cube's current rotation
	model := mgl32.Ident4()
	model = model.Mul4(mgl32.HomogRotate3DY(a.totalRotationY))
	model = model.Mul4(mgl32.HomogRotate3DX(a.totalRotationX))

	a.drawCube(model)
	a.window.SwapBuffers()
}

// drawCube draws the predefined cube with the given model matrix.
func (a *AppCore) drawCube(modelMatrix mgl32.Mat4) {
	gl.UniformMatrix4fv(a.modelUniform, 1, false, &modelMatrix[0])

	gl.BindVertexArray(a.vao)
	gl.DrawElements(gl.TRIANGLES, a.indicesCount, gl.UNSIGNED_INT, unsafe.Pointer(uintptr(0)))
	gl.BindVertexArray(0)
}

// updateAndDisplayFPS calculates and displays FPS in the window title.
func (a *AppCore) updateAndDisplayFPS() {
	a.fpsFrames++
	if time.Since(a.fpsLastUpdateTime) >= time.Second {
		fps := float64(a.fpsFrames) / time.Since(a.fpsLastUpdateTime).Seconds()
		a.window.SetTitle(fmt.Sprintf("%s | FPS: %.2f", a.title, fps)) // Use original title + FPS
		a.fpsFrames = 0
		a.fpsLastUpdateTime = time.Now()
	}
}

// shutdownApp cleans up core OpenGL and GLFW resources.
func shutdownApp() {
	if app == nil { // Ensure app is initialized before attempting to clean up
		return
	}
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

// shouldClose returns true if the window should close.
func (a *AppCore) shouldClose() bool {
	return a.window.ShouldClose() || !a.running
}

// main orchestrates the application flow using high-level functions.
func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer shutdownApp()

	if err := initApp(); err != nil {
		log.Fatalf("Application initialization failed: %v", err)
	}

	log.Println("Engine initialized. Starting main loop...")

	// Main Game Loop
	for !app.shouldClose() {
		// 1. Event Processing (includes VSync toggle)
		app.processInput()

		// Calculate delta time
		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(app.lastFrameTime).Seconds())
		app.lastFrameTime = currentTime

		// 2. Update Game Logic
		app.updateScene(deltaTime)

		// 3. Render Scene
		app.renderScene()

		// 4. Update and Display FPS
		app.updateAndDisplayFPS()
	}

	log.Println("Engine shutting down.")
}
