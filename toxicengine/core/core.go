package core

import (
	"fmt"
	"log"
	"runtime" // Still useful for runtime.LockOSThread in some scenarios, but SDL manages its own thread
	"strings"
	"unsafe" // For gl.PtrOffset

	"github.com/go-gl/gl"            // Still used for raw OpenGL commands
	"github.com/go-gl/mathgl/mgl32" // Still used for matrix math
	"github.com/veandco/go-sdl2/sdl" // New: SDL2 for windowing and events
)

// Core struct encapsulates the low-level graphics and windowing components.
type Core struct {
	window *sdl.Window
	glContext sdl.GLContext // Store the OpenGL context created by SDL

	// OpenGL program and buffers for the cube
	program        uint32
	vao            uint32
	vbo            uint32
	ebo            uint32
	indicesCount   int32

	// Uniform locations
	modelUniform      int32
	viewUniform       int32
	projectionUniform int32

	// Window dimensions
	width, height int
	title         string

	// Internal state for main loop
	running bool

	// Cube data (for now)
	vertices []float32
	indices  []uint32
}

// NewCore creates and initializes a new Core graphics instance.
func NewCore(width, height int, title string) *Core {
	c := &Core{
		width:   width,
		height:  height,
		title:   title,
		running: true, // Start as running
	}

	c.vertices = []float32{
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
	c.indices = []uint32{
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
	c.indicesCount = int32(len(c.indices))

	return c
}

// Init initializes SDL, OpenGL context, and prepares the core rendering data.
func (c *Core) Init() error {
	// SDL doesn't usually require runtime.LockOSThread() as GLFW does
	// for its main loop.
	// runtime.LockOSThread()

	if err := sdl.Init(sdl.INIT_EVERYTHING); err != nil {
		return fmt.Errorf("failed to initialize SDL: %w", err)
	}

	// Set OpenGL attributes for context creation
	sdl.GLSetAttribute(sdl.GL_CONTEXT_MAJOR_VERSION, 4)
	sdl.GLSetAttribute(sdl.GL_CONTEXT_MINOR_VERSION, 1)
	sdl.GLSetAttribute(sdl.GL_CONTEXT_PROFILE_MASK, sdl.GL_CONTEXT_PROFILE_CORE)
	sdl.GLSetAttribute(sdl.GL_DOUBLEBUFFER, 1) // Enable double buffering

	window, err := sdl.CreateWindow(c.title, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		int32(c.width), int32(c.height), sdl.WINDOW_OPENGL|sdl.WINDOW_RESIZABLE)
	if err != nil {
		sdl.Quit()
		return fmt.Errorf("failed to create SDL window: %w", err)
	}
	c.window = window

	glContext, err := window.GLCreateContext()
	if err != nil {
		window.Destroy()
		sdl.Quit()
		return fmt.Errorf("failed to create OpenGL context: %w", err)
	}
	c.glContext = glContext
	sdl.GLMakeCurrent(window, glContext) // Make the context current after creation

	if err := gl.Init(); err != nil {
		window.Destroy()
		sdl.Quit()
		return fmt.Errorf("failed to initialize OpenGL: %w", err)
	}

	gl.Enable(gl.DEPTH_TEST)
	gl.Viewport(0, 0, int32(c.width), int32(c.height))

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
		c.window.Destroy()
		sdl.Quit()
		return fmt.Errorf("failed to compile shaders: %w", err)
	}
	gl.UseProgram(program)
	c.program = program

	// Get uniform locations
	c.modelUniform = gl.GetUniformLocation(c.program, gl.Str("model\x00"))
	c.viewUniform = gl.GetUniformLocation(c.program, gl.Str("view\x00"))
	c.projectionUniform = gl.GetUniformLocation(c.program, gl.Str("projection\x00"))

	// --- VBO, VAO, EBO Setup for the cube (unchanged) ---
	gl.GenVertexArrays(1, &c.vao)
	gl.BindVertexArray(c.vao)

	gl.GenBuffers(1, &c.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, c.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(c.vertices)*4, gl.Ptr(c.vertices), gl.STATIC_DRAW)

	gl.GenBuffers(1, &c.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, c.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(c.indices)*4, gl.Ptr(c.indices), gl.STATIC_DRAW)

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
	gl.UniformMatrix4fv(c.viewUniform, 1, false, &view[0])

	// --- Projection Matrix Setup (Perspective) (unchanged) ---
	projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(c.width)/float32(c.height), 0.1, 100.0)
	gl.UniformMatrix4fv(c.projectionUniform, 1, false, &projection[0])

	return nil
}

// ShouldClose returns true if the window should close (based on internal running flag).
func (c *Core) ShouldClose() bool {
	return !c.running
}

// PollEvents processes window events.
func (c *Core) PollEvents() {
	for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
		switch event := event.(type) {
		case *sdl.QuitEvent:
			c.running = false
		case *sdl.KeyboardEvent:
			if event.Type == sdl.KEYDOWN {
				switch event.Keysym.Sym {
				case sdl.K_ESCAPE:
					c.running = false
				}
			}
		// Add other event handlers here as needed (mouse, resize, etc.)
		case *sdl.WindowEvent:
			if event.Event == sdl.WINDOWEVENT_RESIZED {
				// Get current window size after resize
				width, height := c.window.GetSize()
				c.width = int(width)
				c.height = int(height)
				gl.Viewport(0, 0, int32(c.width), int32(c.height))
				// Update projection matrix as aspect ratio might change
				projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(c.width)/float32(c.height), 0.1, 100.0)
				gl.UniformMatrix4fv(c.projectionUniform, 1, false, &projection[0])
			}
		}
	}
}

// ClearFrame clears the color and depth buffers.
func (c *Core) ClearFrame() {
	gl.ClearColor(0.2, 0.3, 0.3, 1.0) // Dark teal background
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
}

// SwapBuffers swaps the front and back buffers to display the rendered frame.
func (c *Core) SwapBuffers() {
	c.window.GLSwap() // SDL equivalent of glfw.SwapBuffers
}

// DrawCube draws the predefined cube with the given model matrix.
func (c *Core) DrawCube(modelMatrix mgl32.Mat4) {
	gl.UniformMatrix4fv(c.modelUniform, 1, false, &modelMatrix[0])

	gl.BindVertexArray(c.vao)
	gl.DrawElements(gl.TRIANGLES, c.indicesCount, gl.UNSIGNED_INT, unsafe.Pointer(uintptr(0)))
	gl.BindVertexArray(0)
}

// Shutdown cleans up core OpenGL and SDL resources.
func (c *Core) Shutdown() {
	gl.DeleteVertexArrays(1, &c.vao)
	gl.DeleteBuffers(1, &c.vbo)
	gl.DeleteBuffers(1, &c.ebo)
	gl.DeleteProgram(c.program)

	if c.glContext != nil {
		sdl.GLDeleteContext(c.glContext)
	}
	if c.window != nil {
		c.window.Destroy()
	}
	sdl.Quit() // Quit SDL subsystems

	// runtime.UnlockOSThread() // No longer needed as we removed runtime.LockOSThread()
}

// --- Helper functions for shader compilation (unchanged, stay in core.go) ---

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
