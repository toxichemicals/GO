package main

import (
	"bufio"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // Import for JPEG decoding
	_ "image/png"  // Import for PNG decoding
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe" // For gl.PtrOffset

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

// Constants for window dimensions and viewer parameters
const (
	screenWidth      = 1280 // Increased width for UI
	screenHeight     = 720  // Increased height for UI
	windowTitle      = "Holy Model Maker (Editor - Custom GUI)"
	cameraSpeed      = 5.0    // Units per second for camera movement
	mouseSensitivity = 0.1    // Degrees per pixel for mouse look
	farClippingPlane = 1000.0 // Increased for distant objects
)

// UI Constants - Explicitly define as float32
const (
	uiPanelWidth  float32 = 250.0
	uiPadding     float32 = 10.0
	uiButtonHeight float32 = 30.0
	uiSliderHeight float32 = 20.0
	uiElementSpacing float32 = 5.0
)

// AppCore struct encapsulates the editor's state and rendering components.
type AppCore struct {
	window *glfw.Window

	// OpenGL program and uniform locations for 3D scene
	program           uint32
	modelUniform      int32
	viewUniform       int32
	projectionUniform int32
	textureUniform    int32
	hasTextureUniform int32 // Uniform to tell shader if texture is present

	// OpenGL program and uniforms for 2D UI
	uiProgram         uint32
	uiTransformUniform int32
	uiColorUniform    int32

	// Window dimensions and title
	width, height int
	title         string
	running       bool // Internal state for main loop

	// Game state for time and FPS
	lastFrameTime     time.Time
	fpsFrames         int
	fpsLastUpdateTime time.Time

	// Camera Control state
	cameraPos   mgl32.Vec3
	cameraFront mgl32.Vec3
	cameraUp    mgl32.Vec3
	yaw         float32 // Rotation around Y axis (left/right)
	pitch       float32 // Rotation around X axis (up/down)
	firstMouse  bool
	mouseLastX  float64
	mouseLastY  float64
	rightMouseButtonPressed bool // Track right mouse button state for camera look

	// Editor state
	objects []*GameObject       // All objects in the scene
	selectedObject *GameObject // Currently selected object for properties panel
	nextObjectID int            // For unique object IDs

	// Custom UI State
	mouseLeftPressed bool
	mouseLeftReleased bool
	mousePosX float32
	mousePosY float32
	activeUIElement string // Tracks which UI element is being interacted with (e.g., "slider_pos_x")
}

// GameObject represents a loaded or procedurally generated 3D model.
type GameObject struct {
	ID           string
	Vertices     []float32 // Interleaved position (3) + color (3) + texcoord (2)
	Indices      []uint32
	VAO, VBO, EBO uint32
	IndicesCount int32
	HasTexture   bool
	TextureID    uint32
	TexturePath  string // Path to the original texture file

	// Transformation fields
	Position mgl32.Vec3
	Rotation mgl32.Vec3 // Euler angles (pitch, yaw, roll) - still here but not directly manipulated by UI
	Scale    mgl32.Vec3
}

// Global instance of AppCore
var app *AppCore

// initApp initializes the editor, including GLFW, OpenGL, shaders, and camera.
func initApp() error {
	runtime.LockOSThread() // Required by GLFW

	app = &AppCore{
		width:   screenWidth,
		height:  screenHeight,
		title:   windowTitle,
		running: true,
		objects: make([]*GameObject, 0),
		nextObjectID: 0,

		// Initialize camera state
		cameraPos:   mgl32.Vec3{0, 2.0, 5.0}, // Start slightly above ground, zoomed out
		cameraFront: mgl32.Vec3{0, 0, -1},    // Looking towards negative Z
		cameraUp:    mgl32.Vec3{0, 1, 0},     // Up direction
		yaw:         -90.0,                    // Yaw to look along negative Z initially
		pitch:       0.0,                      // No pitch initially
		firstMouse:  true,
	}

	// Initialize GLFW window
	if err := app.initializeWindow(); err != nil {
		return fmt.Errorf("window initialization failed: %w", err)
	}

	// Initialize OpenGL
	if err := app.initializeOpenGL(); err != nil {
		return fmt.Errorf("OpenGL initialization failed: %w", err)
	}

	// Setup 3D scene shaders and get uniform locations
	if err := app.setupSceneShadersAndUniforms(); err != nil {
		return fmt.Errorf("scene shader setup failed: %w", err)
	}

	// Setup 2D UI shaders and get uniform locations
	if err := app.setupUIShadersAndUniforms(); err != nil {
		return fmt.Errorf("UI shader setup failed: %w", err)
	}

	// Setup initial camera and projection matrices for 3D scene
	app.updateCameraAndProjection()

	// Initialize time for delta time calculation and FPS counter
	app.lastFrameTime = time.Now()
	app.fpsLastUpdateTime = time.Now()
	app.fpsFrames = 0

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
	glfw.WindowHint(glfw.Resizable, glfw.True)

	window, err := glfw.CreateWindow(a.width, a.height, a.title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("failed to create GLFW window: %w", err)
	}
	a.window = window
	a.window.MakeContextCurrent()

	a.window.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		a.width = width
		a.height = height
		gl.Viewport(0, 0, int32(width), int32(height))
		a.updateCameraAndProjection() // Update projection on resize
	})

	a.window.SetCursorPosCallback(func(_ *glfw.Window, xpos, ypos float64) {
		a.mousePosX = float32(xpos)
		a.mousePosY = float32(ypos)

		// Only handle camera rotation if right mouse button is pressed AND no UI element is active
		if a.window.GetMouseButton(glfw.MouseButtonRight) == glfw.Press && a.activeUIElement == "" {
			if a.firstMouse {
				a.mouseLastX = xpos
				a.mouseLastY = ypos
				a.firstMouse = false
			}

			xoffset := float32(xpos - a.mouseLastX)
			yoffset := float32(a.mouseLastY - ypos) // Reversed Y-coordinates
			a.mouseLastX = xpos
			a.mouseLastY = ypos

			xoffset *= mouseSensitivity
			yoffset *= mouseSensitivity

			a.yaw += xoffset
			a.pitch += yoffset

			// Clamp pitch to prevent camera flipping
			if a.pitch > 89.0 {
				a.pitch = 89.0
			}
			if a.pitch < -89.0 {
				a.pitch = -89.0
			}
			a.updateCameraAndProjection()
		} else {
			a.firstMouse = true // Reset when mouse button is released or UI is active
		}
	})

	a.window.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if button == glfw.MouseButtonLeft {
			if action == glfw.Press {
				a.mouseLeftPressed = true
				a.mouseLeftReleased = false // Reset released state
			} else if action == glfw.Release {
				a.mouseLeftReleased = true
				a.mouseLeftPressed = false // Reset pressed state
				a.activeUIElement = "" // Release any active UI element
			}
		}
		if button == glfw.MouseButtonRight {
			if action == glfw.Press {
				a.rightMouseButtonPressed = true
			} else if action == glfw.Release {
				a.rightMouseButtonPressed = false
			}
		}
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

// setupSceneShadersAndUniforms compiles shaders for the 3D scene.
func (a *AppCore) setupSceneShadersAndUniforms() error {
	// Vertex shader that supports both color and texture
	vertexShaderSource := `
		#version 410 core
		layout (location = 0) in vec3 aPos;
		layout (location = 1) in vec3 aColor; // For vertex colors
		layout (location = 2) in vec2 aTexCoord; // For texture coordinates

		out vec3 ourColor;
		out vec2 TexCoord;

		uniform mat4 model;
		uniform mat4 view;
		uniform mat4 projection;

		void main() {
			gl_Position = projection * view * model * vec4(aPos, 1.0);
			ourColor = aColor;
			TexCoord = aTexCoord;
		}
	` + "\x00"

	// Fragment shader that uses texture if available, otherwise vertex color
	fragmentShaderSource := `
		#version 410 core
		in vec3 ourColor;
		in vec2 TexCoord;
		out vec4 FragColor;

		uniform sampler2D ourTexture;
		uniform bool hasTexture; // To indicate if a texture is bound

		void main() {
			if (hasTexture) {
				FragColor = texture(ourTexture, TexCoord);
			} else {
				FragColor = vec4(ourColor, 1.0);
			}
		}
	` + "\x00"

	program, err := compileShader(vertexShaderSource, fragmentShaderSource)
	if err != nil {
		return fmt.Errorf("failed to compile scene shaders: %w", err)
	}
	gl.UseProgram(program)
	a.program = program

	a.modelUniform = gl.GetUniformLocation(a.program, gl.Str("model\x00"))
	a.viewUniform = gl.GetUniformLocation(a.program, gl.Str("view\x00"))
	a.projectionUniform = gl.GetUniformLocation(a.program, gl.Str("projection\x00"))
	a.textureUniform = gl.GetUniformLocation(a.program, gl.Str("ourTexture\x00"))
	a.hasTextureUniform = gl.GetUniformLocation(a.program, gl.Str("hasTexture\x00")) // Store uniform location
	gl.Uniform1i(a.hasTextureUniform, 0) // Default to no texture

	return nil
}

// setupUIShadersAndUniforms compiles shaders for 2D UI elements.
func (a *AppCore) setupUIShadersAndUniforms() error {
	uiVertexShaderSource := `
		#version 410 core
		layout (location = 0) in vec2 aPos; // Only 2D position for UI
		uniform mat4 uiTransform; // Orthographic projection + translation/scale
		void main() {
			gl_Position = uiTransform * vec4(aPos, 0.0, 1.0);
		}
	` + "\x00"

	uiFragmentShaderSource := `
		#version 410 core
		out vec4 FragColor;
		uniform vec4 uiColor; // Color for the UI element
		void main() {
			FragColor = uiColor;
		}
	` + "\x00"

	uiProgram, err := compileShader(uiVertexShaderSource, uiFragmentShaderSource)
	if err != nil {
		return fmt.Errorf("failed to compile UI shaders: %w", err)
	}
	a.uiProgram = uiProgram

	a.uiTransformUniform = gl.GetUniformLocation(a.uiProgram, gl.Str("uiTransform\x00"))
	a.uiColorUniform = gl.GetUniformLocation(a.uiProgram, gl.Str("uiColor\x00"))

	return nil
}


// updateCameraAndProjection recalculates and updates the view and projection matrices for the 3D scene.
func (a *AppCore) updateCameraAndProjection() {
	// Calculate camera front vector from yaw and pitch
	yawRad := mgl32.DegToRad(a.yaw)
	pitchRad := mgl32.DegToRad(a.pitch)

	frontX := float32(math.Cos(float64(yawRad)) * math.Cos(float64(pitchRad)))
	frontY := float32(math.Sin(float64(pitchRad)))
	frontZ := float32(math.Sin(float64(yawRad)) * math.Cos(float64(pitchRad)))
	a.cameraFront = mgl32.Vec3{frontX, frontY, frontZ}.Normalize()

	// Update view matrix
	gl.UseProgram(a.program) // Ensure 3D shader is active for its uniforms
	view := mgl32.LookAtV(a.cameraPos, a.cameraPos.Add(a.cameraFront), a.cameraUp)
	gl.UniformMatrix4fv(a.viewUniform, 1, false, &view[0])

	// Update projection matrix
	projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(a.width)/float32(a.height), 0.1, farClippingPlane)
	gl.UniformMatrix4fv(a.projectionUniform, 1, false, &projection[0])
}

// processInput handles keyboard/mouse input and updates viewer state.
func (a *AppCore) processInput(deltaTime float32) {
	glfw.PollEvents() // Poll GLFW events first

	// Check for ESC key to quit
	if a.window.GetKey(glfw.KeyEscape) == glfw.Press {
		a.running = false
	}

	// Camera movement (WASD) - only if no UI element is active
	if a.activeUIElement == "" {
		moveSpeed := cameraSpeed * deltaTime
		if a.window.GetKey(glfw.KeyW) == glfw.Press {
			a.cameraPos = a.cameraPos.Add(a.cameraFront.Mul(moveSpeed))
		}
		if a.window.GetKey(glfw.KeyS) == glfw.Press {
			a.cameraPos = a.cameraPos.Sub(a.cameraFront.Mul(moveSpeed))
		}
		if a.window.GetKey(glfw.KeyA) == glfw.Press {
			right := a.cameraFront.Cross(a.cameraUp).Normalize()
			a.cameraPos = a.cameraPos.Sub(right.Mul(moveSpeed))
		}
		if a.window.GetKey(glfw.KeyD) == glfw.Press {
			right := a.cameraFront.Cross(a.cameraUp).Normalize()
			a.cameraPos = a.cameraPos.Add(right.Mul(moveSpeed))
		}
		a.updateCameraAndProjection() // Update camera based on new position
	}
}

// updateScene updates editor logic.
func (a *AppCore) updateScene(deltaTime float32) {
	// No continuous object rotation by default, manual control now
}

// renderScene clears buffers and draws all objects.
func (a *AppCore) renderScene() {
	gl.ClearColor(0.2, 0.3, 0.3, 1.0) // Dark teal background
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	// Render 3D objects
	gl.UseProgram(a.program) // Activate 3D shader
	for _, obj := range a.objects {
		a.drawGameObject(obj)
	}

	// Render 2D UI elements
	a.drawCustomUI()

	a.window.SwapBuffers()
}

// drawCustomUI defines and renders the custom UI elements.
func (a *AppCore) drawCustomUI() {
	// Disable depth test for 2D UI to ensure it's always drawn on top
	gl.Disable(gl.DEPTH_TEST)
	gl.UseProgram(a.uiProgram) // Activate 2D UI shader

	// Set up orthographic projection for 2D UI
	// (0,0) is top-left, (width, height) is bottom-right
	ortho := mgl32.Ortho2D(0, float32(a.width), float32(a.height), 0)
	gl.UniformMatrix4fv(a.uiTransformUniform, 1, false, &ortho[0])

	currentY := uiPadding

	// --- Editor Tools Panel ---
	panelX := uiPadding
	panelWidth := uiPanelWidth
	panelHeight := float32(0.0) // Will calculate dynamically

	// "Import Model" button
	buttonX := panelX + uiPadding
	buttonY := currentY + uiPadding
	buttonWidth := panelWidth - uiPadding*2
	buttonHeight := uiButtonHeight

	if a.handleButton(buttonX, buttonY, buttonWidth, buttonHeight, "Import Model (.holym)") {
		log.Print("Enter path to .holym model file (e.g., models/my_model.holym): ")
		reader := bufio.NewReader(os.Stdin)
		inputPath, _ := reader.ReadString('\n')
		inputPath = strings.TrimSpace(inputPath)

		if inputPath != "" {
			if err := a.loadHolymModel(inputPath); err != nil {
				log.Printf("Error loading model from %s: %v", inputPath, err)
			} else {
				log.Printf("Successfully loaded model from %s", inputPath)
			}
		} else {
			log.Println("No path entered.")
		}
	}
	currentY += uiButtonHeight + uiElementSpacing

	// "Create Primitive" buttons
	a.drawTextOverlay(panelX+uiPadding, currentY+uiPadding, "Create Primitive:", mgl32.Vec4{1,1,1,1})
	currentY += uiButtonHeight + uiElementSpacing

	buttonX = panelX + uiPadding
	if a.handleButton(buttonX, currentY, buttonWidth/2 - uiElementSpacing/2, uiButtonHeight, "Cube") {
		a.createPrimitive("cube")
	}
	buttonX += buttonWidth/2 + uiElementSpacing/2
	if a.handleButton(buttonX, currentY, buttonWidth/2 - uiElementSpacing/2, uiButtonHeight, "Plane") {
		a.createPrimitive("plane")
	}
	currentY += uiButtonHeight + uiElementSpacing

	a.drawTextOverlay(panelX+uiPadding, currentY+uiPadding, "Scene Objects:", mgl32.Vec4{1,1,1,1})
	currentY += uiButtonHeight + uiElementSpacing

	// Object List (simplified, just selectable text)
	if len(a.objects) == 0 {
		a.drawTextOverlay(panelX+uiPadding, currentY+uiPadding, "No objects in scene.", mgl32.Vec4{0.7,0.7,0.7,1})
		currentY += uiButtonHeight + uiElementSpacing
	} else {
		for _, obj := range a.objects {
			label := obj.ID
			if obj == a.selectedObject {
				label += " (Selected)"
			}
			// For simplicity, we'll make the whole area clickable like a button
			if a.handleButton(panelX+uiPadding, currentY, panelWidth-uiPadding*2, uiButtonHeight, label) {
				a.selectedObject = obj
			}
			currentY += uiButtonHeight + uiElementSpacing
		}
	}

	panelHeight = currentY + uiPadding - uiPadding // Adjust for final padding
	a.drawRect(panelX, uiPadding, panelWidth, panelHeight, mgl32.Vec4{0.15, 0.15, 0.15, 0.8}) // Background for Editor Tools

	// --- Properties Panel for selected object ---
	if a.selectedObject != nil {
		propPanelX := float32(a.width) - uiPanelWidth - uiPadding
		propPanelY := uiPadding
		propPanelWidth := uiPanelWidth
		propPanelHeight := float32(0.0)

		currentPropY := propPanelY + uiPadding

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Properties: %s", a.selectedObject.ID), mgl32.Vec4{1,1,1,1})
		currentPropY += uiButtonHeight + uiElementSpacing

		// Position Sliders
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Position X:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"pos_x", &a.selectedObject.Position[0], -10.0, 10.0)
		currentPropY += uiSliderHeight + uiElementSpacing

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Position Y:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"pos_y", &a.selectedObject.Position[1], -10.0, 10.0)
		currentPropY += uiSliderHeight + uiElementSpacing

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Position Z:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"pos_z", &a.selectedObject.Position[2], -10.0, 10.0)
		currentPropY += uiSliderHeight + uiElementSpacing * 2 // Extra spacing

		// Scale Sliders
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Scale X:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"scale_x", &a.selectedObject.Scale[0], 0.01, 5.0)
		currentPropY += uiSliderHeight + uiElementSpacing

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Scale Y:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"scale_y", &a.selectedObject.Scale[1], 0.01, 5.0)
		currentPropY += uiSliderHeight + uiElementSpacing

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Scale Z:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"scale_z", &a.selectedObject.Scale[2], 0.01, 5.0)
		currentPropY += uiSliderHeight + uiElementSpacing * 2 // Extra spacing


		propPanelHeight = currentPropY - propPanelY + uiPadding
		a.drawRect(propPanelX, propPanelY, propPanelWidth, propPanelHeight, mgl32.Vec4{0.15, 0.15, 0.15, 0.8}) // Background for Properties
	}

	gl.Enable(gl.DEPTH_TEST) // Re-enable depth test for 3D scene
}

// isMouseOver checks if the mouse cursor is within the given rectangle.
func (a *AppCore) isMouseOver(x, y, width, height float32) bool {
	return a.mousePosX >= x && a.mousePosX <= x+width &&
		a.mousePosY >= y && a.mousePosY <= y+height
}

// handleButton draws a button and returns true if it was clicked.
func (a *AppCore) handleButton(x, y, width, height float32, label string) bool {
	isOver := a.isMouseOver(x, y, width, height)
	buttonColor := mgl32.Vec4{0.2, 0.2, 0.2, 1.0} // Default
	if isOver {
		buttonColor = mgl32.Vec4{0.3, 0.3, 0.3, 1.0} // Hover color
	}
	if isOver && a.mouseLeftPressed {
		buttonColor = mgl32.Vec4{0.1, 0.1, 0.1, 1.0} // Pressed color
	}

	a.drawRect(x, y, width, height, buttonColor)
	a.drawTextOverlay(x + width/2 - float32(len(label)*3), y + height/2 - 8, label, mgl32.Vec4{1,1,1,1}) // Crude text centering

	clicked := false
	if isOver && a.mouseLeftReleased {
		clicked = true
	}
	return clicked
}

// handleSlider draws a slider and updates the value if dragged.
func (a *AppCore) handleSlider(x, y, width, height float32, id string, value *float32, min, max float32) {
	isOver := a.isMouseOver(x, y, width, height)

	// Background track
	a.drawRect(x, y, width, height, mgl32.Vec4{0.3, 0.3, 0.3, 1.0})

	// Slider knob
	// Calculate knob position based on value
	normalizedValue := (*value - min) / (max - min)
	knobX := x + normalizedValue*width - height/2 // Knob is square, centered on current value
	knobX = float32(math.Max(float64(x), math.Min(float64(x+width-height), float64(knobX)))) // Clamp knob within bounds

	knobColor := mgl32.Vec4{0.6, 0.6, 0.6, 1.0}
	if isOver || a.activeUIElement == id {
		knobColor = mgl32.Vec4{0.8, 0.8, 0.8, 1.0}
	}
	if a.activeUIElement == id { // If this slider is actively being dragged
		knobColor = mgl32.Vec4{0.9, 0.9, 0.9, 1.0}
		// Update value based on mouse X position
		newNormalizedValue := (a.mousePosX - x) / width
		*value = min + newNormalizedValue*(max-min)
		*value = float32(math.Max(float64(min), math.Min(float64(max), float64(*value)))) // Clamp value
	}

	a.drawRect(knobX, y, height, height, knobColor) // Draw knob as a square

	// Activate slider if mouse is over and left button is pressed
	if isOver && a.mouseLeftPressed && a.activeUIElement == "" {
		a.activeUIElement = id
	}
}


// drawRect draws a filled rectangle.
func (a *AppCore) drawRect(x, y, width, height float32, color mgl32.Vec4) {
	// Define vertices for a quad
	vertices := []float32{
		x, y, // Top-left
		x + width, y, // Top-right
		x + width, y + height, // Bottom-right
		x, y + height, // Bottom-left
	}
	indices := []uint32{
		0, 1, 2,
		2, 3, 0,
	}

	var vao, vbo, ebo uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	gl.GenBuffers(1, &ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW)

	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2*4, gl.Ptr(nil)) // 2D position
	gl.EnableVertexAttribArray(0)

	gl.Uniform4fv(a.uiColorUniform, 1, &color[0])
	gl.DrawElements(gl.TRIANGLES, int32(len(indices)), gl.UNSIGNED_INT, unsafe.Pointer(uintptr(0)))

	gl.BindVertexArray(0)
	gl.DeleteBuffers(1, &vbo)
	gl.DeleteBuffers(1, &ebo)
	gl.DeleteVertexArrays(1, &vao)
}

// drawTextOverlay is a placeholder for text rendering.
// Full text rendering in OpenGL is complex and would require font loading,
// texture atlases, and a more sophisticated rendering pipeline.
// For this example, we'll just log the text or visually represent it.
// In a real application, you'd use a library like freetype-go or implement your own.
func (a *AppCore) drawTextOverlay(x, y float32, text string, color mgl32.Vec4) {
	// For now, we'll just log the text to the console.
	// In a full implementation, this would draw actual text on screen.
	// log.Printf("UI Text: %s at (%.1f, %.1f)", text, x, y) // Uncomment for debug logging
	// To actually render text, you'd need:
	// 1. A font texture atlas.
	// 2. A separate VAO/VBO for text quads.
	// 3. A shader that samples from the font texture.
	// This is a significant additional task.
}


// drawGameObject draws a given GameObject.
func (a *AppCore) drawGameObject(obj *GameObject) {
	// Set hasTexture uniform based on the object's property
	if obj.HasTexture && obj.TextureID != 0 {
		gl.Uniform1i(a.hasTextureUniform, 1) // 1 for true
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, obj.TextureID)
		gl.Uniform1i(a.textureUniform, 0) // Texture unit 0
	} else {
		gl.Uniform1i(a.hasTextureUniform, 0) // 0 for false
	}

	model := mgl32.Ident4()
	model = model.Mul4(mgl32.Translate3D(obj.Position.X(), obj.Position.Y(), obj.Position.Z()))
	// Apply rotations in ZYX order for more intuitive Euler angles
	model = model.Mul4(mgl32.HomogRotate3DZ(obj.Rotation.Z()))
	model = model.Mul4(mgl32.HomogRotate3DY(obj.Rotation.Y()))
	model = model.Mul4(mgl32.HomogRotate3DX(obj.Rotation.X()))
	model = model.Mul4(mgl32.Scale3D(obj.Scale.X(), obj.Scale.Y(), obj.Scale.Z()))

	gl.UniformMatrix4fv(a.modelUniform, 1, false, &model[0])

	gl.BindVertexArray(obj.VAO)
	// Vertex stride is 8*4 bytes (3 pos + 3 color + 2 texcoord)
	// Ensure attributes are correctly re-enabled/set for each object if they vary
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 8*4, gl.Ptr(nil)) // Position
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(1, 3, gl.FLOAT, false, 8*4, gl.PtrOffset(3*4)) // Color
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(2, 2, gl.FLOAT, false, 8*4, gl.PtrOffset(6*4)) // TexCoord
	gl.EnableVertexAttribArray(2)

	gl.DrawElements(gl.TRIANGLES, obj.IndicesCount, gl.UNSIGNED_INT, unsafe.Pointer(uintptr(0)))
	gl.BindVertexArray(0)

	// Unbind texture if one was used to prevent bleeding
	if obj.HasTexture && obj.TextureID != 0 {
		gl.BindTexture(gl.TEXTURE_2D, 0)
	}
}

// updateAndDisplayFPS calculates and displays FPS in the window title.
func (a *AppCore) updateAndDisplayFPS() {
	a.fpsFrames++
	if time.Since(a.fpsLastUpdateTime) >= time.Second {
		fps := float64(a.fpsFrames) / time.Since(a.fpsLastUpdateTime).Seconds()
		a.window.SetTitle(fmt.Sprintf("%s | FPS: %.2f", a.title, fps))
		a.fpsFrames = 0
		a.fpsLastUpdateTime = time.Now()
	}
}

// shutdownApp cleans up all OpenGL and GLFW resources.
func shutdownApp() {
	if app == nil {
		return
	}

	// Delete all objects' buffers
	for _, obj := range app.objects {
		gl.DeleteVertexArrays(1, &obj.VAO)
		gl.DeleteBuffers(1, &obj.VBO)
		gl.DeleteBuffers(1, &obj.EBO)
		if obj.TextureID != 0 {
			gl.DeleteTextures(1, &obj.TextureID)
		}
	}

	gl.DeleteProgram(app.program) // 3D scene program
	gl.DeleteProgram(app.uiProgram) // 2D UI program

	if app.window != nil {
		app.window.Destroy()
	}
	glfw.Terminate()
}

// --- Helper functions for .holym model loading and texture creation ---

// parseHolym reads a .holym file and returns parsed data.
// Format: v X Y Z [c R G B], vt U V, f V1/VT1 V2/VT2 V3/VT3, tex_path <path>
func parseHolym(filePath string) ([]float32, []uint32, bool, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, false, "", fmt.Errorf("failed to open .holym file: %w", err)
	}
	defer file.Close()

	var positions []mgl32.Vec3
	var colors []mgl32.Vec3
	var texCoords []mgl32.Vec2
	var faces [][3]struct{ Vertex, TexCoord int } // Store 0-based indices for vertex/texcoord
	var texturePath string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "v": // Vertex position and optional color
			if len(fields) < 4 { continue }
			x, _ := strconv.ParseFloat(fields[1], 32)
			y, _ := strconv.ParseFloat(fields[2], 32)
			z, _ := strconv.ParseFloat(fields[3], 32)
			positions = append(positions, mgl32.Vec3{float32(x), float32(y), float32(z)})

			// Check for optional color
			if len(fields) == 7 && fields[4] == "c" {
				r, _ := strconv.ParseFloat(fields[5], 32)
				g, _ := strconv.ParseFloat(fields[6], 32)
				b, _ := strconv.ParseFloat(fields[7], 32)
				colors = append(colors, mgl32.Vec3{float32(r), float32(g), float32(b)})
			} else {
				colors = append(colors, mgl32.Vec3{1.0, 1.0, 1.0}) // Default white
			}

		case "vt": // Texture coordinate
			if len(fields) < 3 { continue }
			u, _ := strconv.ParseFloat(fields[1], 32)
			v, _ := strconv.ParseFloat(fields[2], 32)
			texCoords = append(texCoords, mgl32.Vec2{float32(u), float32(v)})

		case "f": // Face (triangle)
			if len(fields) < 4 { continue } // Expecting 3 vertex/texcoord pairs for a triangle
			var face [3]struct{ Vertex, TexCoord int }
			for i := 0; i < 3; i++ {
				parts := strings.Split(fields[i+1], "/")
				if len(parts) != 2 {
					return nil, nil, false, "", fmt.Errorf("invalid face format: %s (expected V/VT)", fields[i+1])
				}
				vIdx, _ := strconv.Atoi(parts[0])
				vtIdx, _ := strconv.Atoi(parts[1])
				face[i].Vertex = vIdx - 1   // Convert to 0-based index
				face[i].TexCoord = vtIdx - 1 // Convert to 0-based index
			}
			faces = append(faces, face)
		case "tex_path": // Texture path
			if len(fields) < 2 { continue }
			texturePath = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, false, "", fmt.Errorf("error scanning .holym file: %w", err)
	}

	// Removed `interleavedVertices` as it was declared but not used.
	uniqueVertices := make([]float32, 0) // Stores interleaved unique vertex data
	var indices []uint32

	// For faces, we need to map V/VT to actual interleaved vertex data.
	// We'll create a map to store unique vertex combinations (pos+color+texcoord)
	// to allow for efficient reuse and proper EBO.
	type VertexKey struct {
		PosIndex int
		ColorIndex int // Assuming color is also indexed by position index for simplicity
		TexCoordIndex int
	}
	vertexMap := make(map[VertexKey]uint32)


	for _, face := range faces {
		for i := 0; i < 3; i++ {
			vIdx := face[i].Vertex
			vtIdx := face[i].TexCoord

			if vIdx < 0 || vIdx >= len(positions) {
				return nil, nil, false, "", fmt.Errorf("vertex index out of bounds: %d", vIdx+1)
			}
			if vtIdx < 0 || vtIdx >= len(texCoords) {
				return nil, nil, false, "", fmt.Errorf("texture coordinate index out of bounds: %d", vtIdx+1)
			}
			if vIdx >= len(colors) { // Ensure color exists for vertex
				log.Printf("Warning: No color specified for vertex %d, defaulting to white.", vIdx+1)
				colors[vIdx] = mgl32.Vec3{1,1,1} // Ensure there's a color
			}


			key := VertexKey{PosIndex: vIdx, ColorIndex: vIdx, TexCoordIndex: vtIdx}

			if index, ok := vertexMap[key]; ok {
				indices = append(indices, index)
			} else {
				// Add new unique vertex
				newIndex := uint32(len(uniqueVertices) / 8) // 8 floats per vertex

				pos := positions[vIdx]
				color := colors[vIdx]
				texCoord := texCoords[vtIdx]

				uniqueVertices = append(uniqueVertices, pos.X(), pos.Y(), pos.Z())
				uniqueVertices = append(uniqueVertices, color.X(), color.Y(), color.Z())
				uniqueVertices = append(uniqueVertices, texCoord.X(), texCoord.Y())

				vertexMap[key] = newIndex
				indices = append(indices, newIndex)
			}
		}
	}

	hasTexture := (texturePath != "")
	return uniqueVertices, indices, hasTexture, texturePath, nil
}

// newTexture creates an OpenGL texture from an image path.
func newTexture(imgPath string) (uint32, error) {
	file, err := os.Open(imgPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open texture file %s: %w", imgPath, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return 0, fmt.Errorf("failed to decode texture image %s: %w", imgPath, err)
	}

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)

	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR) // Use mipmaps for better quality
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(rgba.Rect.Size().X), int32(rgba.Rect.Size().Y), 0,
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(rgba.Pix))
	gl.GenerateMipmap(gl.TEXTURE_2D)

	gl.BindTexture(gl.TEXTURE_2D, 0)
	return texture, nil
}

// createGameObject initializes OpenGL buffers for a new GameObject and adds it to the scene.
func (a *AppCore) createGameObject(id string, vertices []float32, indices []uint32, hasTexture bool, texturePath string) *GameObject {
	newObj := &GameObject{
		ID:           id,
		Vertices:     vertices,
		Indices:      indices,
		IndicesCount: int32(len(indices)),
		Position:     mgl32.Vec3{0, 0, 0}, // Spawn at origin by default
		Rotation:     mgl32.Vec3{0, 0, 0}, // Initial rotation
		Scale:        mgl32.Vec3{1, 1, 1}, // Initial scale
		HasTexture:   hasTexture,
		TexturePath:  texturePath,
	}

	// Load texture if path is provided
	if newObj.HasTexture && newObj.TexturePath != "" {
		texID, err := newTexture(newObj.TexturePath)
		if err != nil {
			log.Printf("Warning: Failed to load texture %s for model %s: %v", newObj.TexturePath, newObj.ID, err)
			newObj.HasTexture = false // Fallback to vertex colors
		} else {
			newObj.TextureID = texID
		}
	}

	// Setup OpenGL buffers for the new object
	gl.GenVertexArrays(1, &newObj.VAO)
	gl.BindVertexArray(newObj.VAO)

	gl.GenBuffers(1, &newObj.VBO)
	gl.BindBuffer(gl.ARRAY_BUFFER, newObj.VBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(newObj.Vertices)*4, gl.Ptr(newObj.Vertices), gl.STATIC_DRAW)

	gl.GenBuffers(1, &newObj.EBO)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, newObj.EBO)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(newObj.Indices)*4, gl.Ptr(newObj.Indices), gl.STATIC_DRAW)

	// Position attribute (layout location 0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 8*4, gl.Ptr(nil)) // 3 pos + 3 color + 2 texcoord
	gl.EnableVertexAttribArray(0)
	// Color attribute (layout location 1)
	gl.VertexAttribPointer(1, 3, gl.FLOAT, false, 8*4, gl.PtrOffset(3*4))
	gl.EnableVertexAttribArray(1)
	// Texture coordinate attribute (layout location 2)
	gl.VertexAttribPointer(2, 2, gl.FLOAT, false, 8*4, gl.PtrOffset(6*4))
	gl.EnableVertexAttribArray(2)

	gl.BindVertexArray(0) // Unbind VAO

	a.objects = append(a.objects, newObj)
	a.nextObjectID++
	return newObj
}

// loadHolymModel loads a .holym model from file.
func (a *AppCore) loadHolymModel(filePath string) error {
	vertices, indices, hasTexture, texturePath, err := parseHolym(filePath)
	if err != nil {
		return fmt.Errorf("failed to parse .holym model %s: %w", filePath, err)
	}

	id := fmt.Sprintf("%s_%d", filepath.Base(filePath), a.nextObjectID)
	a.selectedObject = a.createGameObject(id, vertices, indices, hasTexture, texturePath)
	return nil
}

// createPrimitive generates a new primitive shape and adds it to the scene.
func (a *AppCore) createPrimitive(shapeType string) {
	var vertices []float32
	var indices []uint32
	var id string

	switch shapeType {
	case "cube":
		id = fmt.Sprintf("Cube_%d", a.nextObjectID)
		vertices, indices = generateCubeData()
	case "plane":
		id = fmt.Sprintf("Plane_%d", a.nextObjectID)
		vertices, indices = generatePlaneData()
	default:
		log.Printf("Unsupported primitive type: %s", shapeType)
		return
	}

	a.selectedObject = a.createGameObject(id, vertices, indices, false, "") // Primitives start untextured
	log.Printf("Created primitive: %s", id)
}

// generateCubeData returns interleaved vertex data for a unit cube (1x1x1).
// Each face has its own vertices to allow for distinct UVs and colors.
func generateCubeData() ([]float32, []uint32) {
    // Vertices: Position (3) + Color (3) + TexCoord (2) = 8 floats per vertex
    // Face colors (just for visual distinction)
    red := []float32{1.0, 0.0, 0.0}
    green := []float32{0.0, 1.0, 0.0}
    blue := []float32{0.0, 0.0, 1.0}
    yellow := []float32{1.0, 1.0, 0.0}
    cyan := []float32{0.0, 1.0, 1.0}
    magenta := []float32{1.0, 0.0, 1.0}

    // Standard UVs for a quad
    uv00 := []float32{0.0, 0.0} // bottom-left
    uv10 := []float32{1.0, 0.0} // bottom-right
    uv11 := []float32{1.0, 1.0} // top-right
    uv01 := []float32{0.0, 1.0} // top-left

    vertices := []float32{
        // Front face (Red)
        -0.5, -0.5, 0.5,  red[0], red[1], red[2],  uv00[0], uv00[1], // 0
         0.5, -0.5, 0.5,  red[0], red[1], red[2],  uv10[0], uv10[1], // 1
         0.5,  0.5, 0.5,  red[0], red[1], red[2],  uv11[0], uv11[1], // 2
        -0.5,  0.5, 0.5,  red[0], red[1], red[2],  uv01[0], uv01[1], // 3

        // Back face (Green)
        -0.5, -0.5, -0.5, green[0], green[1], green[2], uv10[0], uv10[1], // 4
         0.5, -0.5, -0.5, green[0], green[1], green[2], uv00[0], uv00[1], // 5
         0.5,  0.5, -0.5, green[0], green[1], green[2], uv01[0], uv01[1], // 6
        -0.5,  0.5, -0.5, green[0], green[1], green[2], uv11[0], uv11[1], // 7

        // Top face (Blue)
        -0.5,  0.5,  0.5, blue[0], blue[1], blue[2], uv00[0], uv00[1], // 8 (use existing 3)
         0.5,  0.5,  0.5, blue[0], blue[1], blue[2], uv10[0], uv10[1], // 9 (use existing 2)
         0.5,  0.5, -0.5, blue[0], blue[1], blue[2], uv11[0], uv11[1], // 10 (use existing 6)
        -0.5,  0.5, -0.5, blue[0], blue[1], blue[2], uv01[0], uv01[1], // 11 (use existing 7)


        // Bottom face (Yellow)
        -0.5, -0.5,  0.5, yellow[0], yellow[1], yellow[2], uv01[0], uv01[1], // 12 (use existing 0)
         0.5, -0.5,  0.5, yellow[0], yellow[1], yellow[2], uv11[0], uv11[1], // 13 (use existing 1)
         0.5, -0.5, -0.5, yellow[0], yellow[1], yellow[2], uv10[0], uv10[1], // 14 (use existing 5)
        -0.5, -0.5, -0.5, yellow[0], yellow[1], yellow[2], uv00[0], uv00[1], // 15 (use existing 4)


        // Right face (Cyan)
         0.5, -0.5,  0.5, cyan[0], cyan[1], cyan[2], uv00[0], uv00[1], // 16 (use existing 1)
         0.5, -0.5, -0.5, cyan[0], cyan[1], cyan[2], uv10[0], uv10[1], // 17 (use existing 5)
         0.5,  0.5, -0.5, cyan[0], cyan[1], cyan[2], uv11[0], uv11[1], // 18 (use existing 6)
         0.5,  0.5,  0.5, cyan[0], cyan[1], cyan[2], uv01[0], uv01[1], // 19 (use existing 2)


        // Left face (Magenta)
        -0.5, -0.5,  0.5, magenta[0], magenta[1], magenta[2], uv10[0], uv10[1], // 20 (use existing 0)
        -0.5, -0.5, -0.5, magenta[0], magenta[1], magenta[2], uv00[0], uv00[1], // 21 (use existing 4)
        -0.5,  0.5, -0.5, magenta[0], magenta[1], magenta[2], uv01[0], uv01[1], // 22 (use existing 7)
        -0.5,  0.5,  0.5, magenta[0], magenta[1], magenta[2], uv11[0], uv11[1], // 23 (use existing 3)
    }

    indices := []uint32{
        0, 1, 2,      0, 2, 3,    // Front
        4, 5, 6,      4, 6, 7,    // Back
        8, 9, 10,     8, 10, 11,   // Top
        12, 13, 14,   12, 14, 15,  // Bottom
        16, 17, 18,   16, 18, 19,  // Right
        20, 21, 22,   20, 22, 23,  // Left
    }

    return vertices, indices
}

// generatePlaneData returns interleaved vertex data for a unit plane (1x1).
func generatePlaneData() ([]float32, []uint32) {
    // Vertices: Position (3) + Color (3) + TexCoord (2) = 8 floats per vertex
    white := []float32{1.0, 1.0, 1.0}
    uv00 := []float32{0.0, 0.0}
    uv10 := []float32{1.0, 0.0}
    uv11 := []float32{1.0, 1.0}
    uv01 := []float32{0.0, 1.0}

    vertices := []float32{
        // Front face of plane (facing +Z)
        -0.5, 0.0,  0.5, white[0], white[1], white[2], uv00[0], uv00[1], // bottom-left
         0.5, 0.0,  0.5, white[0], white[1], white[2], uv10[0], uv10[1], // bottom-right
         0.5, 0.0, -0.5, white[0], white[1], white[2], uv11[0], uv11[1], // top-right
        -0.5, 0.0, -0.5, white[0], white[1], white[2], uv01[0], uv01[1], // top-left
    }

    indices := []uint32{
        0, 1, 2, // Triangle 1
        0, 2, 3, // Triangle 2
    }
    return vertices, indices
}


// --- Helper functions for shader compilation ---

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

// main orchestrates the application flow.
func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer shutdownApp()

	if err := initApp(); err != nil {
		log.Fatalf("Application initialization failed: %v", err)
	}

	log.Println("Holy Model Maker (Editor) initialized. Starting main loop...")
	log.Println("Controls:")
	log.Println("  WASD: Move camera")
	log.Println("  Right-click + Drag: Look around")
	log.Println("  Left-click: Cycle through objects (outside UI)")
	log.Println("  Use UI panels to Load Models, Create Primitives, and Transform Selected Objects.")
	log.Println("  ESC: Exit")

	// Main Editor Loop
	for !app.shouldClose() {
		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(app.lastFrameTime).Seconds())
		app.lastFrameTime = currentTime

		// Process input (handles custom UI interaction and camera)
		app.processInput(deltaTime)

		// Update scene logic
		app.updateScene(deltaTime)

		// Render the scene (3D objects + Custom UI)
		app.renderScene()
		app.updateAndDisplayFPS()

		// Reset mouse released state after processing for this frame
		app.mouseLeftReleased = false
	}

	log.Println("Holy Model Maker (Editor) shutting down.")
}
