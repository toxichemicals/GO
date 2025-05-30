package main

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // Import for JPEG decoding
	_ "image/png"  // Import for PNG decoding
	"log"
	"math"
	"os"
	"runtime"
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
	windowTitle      = "Holy Engine Base" // Changed title
	cameraSpeed      float32 = 5.0    // Units per second for camera movement (Explicitly float32)
	mouseSensitivity = 0.1    // Degrees per pixel for mouse look
	farClippingPlane = 1000.0 // Increased for distant objects

	// Physics constants
	Gravity             = -9.81 // m/s^2, downward acceleration
	PhysicsTimestep     = 1.0 / 60.0 // Fixed timestep for physics updates (60 Hz)
	GroundPlaneY        = 0.0        // Y-coordinate of the ground
	ThrowForceMagnitude = 10.0      // How strong a thrown object is pushed
	PickupRange         = 10.0       // Max distance to pick up an object
	InitialHoldDistance = 2.0        // Default distance to hold object
	MinHoldDistance     = 1.0
	MaxHoldDistance     = 5.0
	ScrollSensitivity   = 0.1 // For adjusting hold distance
)

// UI Constants - Explicitly define as float32
const (
	uiPanelWidth     float32 = 250.0
	uiPadding        float32 = 10.0
	uiButtonHeight   float32 = 30.0
	uiSliderHeight   float32 = 20.0
	uiElementSpacing float32 = 5.0
	uiTextHeight     float32 = 16.0 // Approximate height for a line of text
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
	physicsAccumulator float32 // For fixed physics timestep

	// Camera Control state
	cameraPos   mgl32.Vec3
	cameraFront mgl32.Vec3
	cameraUp    mgl32.Vec3
	yaw         float32 // Rotation around Y axis (left/right)
	pitch       float32 // Rotation around X axis (up/down)
	firstMouse  bool
	mouseLastX  float64
	mouseLastY  float64
	isMouseGrabbed bool // New: Track if mouse is grabbed
	rightMouseButtonPressed bool // Track right mouse button state for camera look

	// Engine state (now more like "engine" state)
	objects []*GameObject       // All objects in the scene
	selectedObject *GameObject // Currently selected object for properties panel
	nextObjectID int            // For unique object IDs

	// Hand tool state
	heldObject *GameObject
	holdDistance float32
	isRotatingHeldObject bool // True if 'R' is held and rotating the object
	lastMouseXForRotation float64
	lastMouseYForRotation float64

	// Custom UI State
	mouseLeftPressed bool // Becomes true on press, false on release
	mouseLeftReleased bool // Becomes true on release, false on next frame
	mousePosX float32
	mousePosY float32
	activeUIElement string // Tracks which UI element is being interacted with (e.g., "slider_pos_x")

	// E GUI state
	isEGUIVisible bool // Controls visibility of the 'E' menu
	eKeyWasPressed bool // Debounce for 'E' key
}

// BoundingBox defines an Axis-Aligned Bounding Box.
type BoundingBox struct {
	Min mgl32.Vec3
	Max mgl32.Vec3
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
	Rotation mgl32.Vec3 // Euler angles (pitch, yaw, roll) for rendering
	Scale    mgl32.Vec3

	// Physics fields
	Velocity      mgl32.Vec3 // Linear velocity
	AngularVelocity mgl32.Vec3 // Angular velocity (radians/second)
	IsKinematic   bool       // If true, object is moved directly, not by physics
	IsGrounded    bool       // True if object is touching the ground
	BoundingBox   BoundingBox // Local-space bounding box
	Mass          float32    // For physics calculations (e.g., momentum)
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
		isMouseGrabbed: true, // Start with mouse grabbed for immediate camera control
		holdDistance: InitialHoldDistance, // Default hold distance
		isEGUIVisible: false, // E GUI is hidden by default
	}

	// Initialize GLFW window
	if err := app.initializeWindow(); err != nil {
		return fmt.Errorf("window initialization failed: %w", err)
	}

	// Set initial mouse mode (grabbed)
	if app.isMouseGrabbed {
		app.window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
	} else {
		app.window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
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

	// Create a ground plane (simple visual, not a full physics collider yet)
	app.createGroundPlane()

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

		// Only handle camera rotation if mouse is grabbed AND not rotating held object AND no UI element is active
		if a.isMouseGrabbed && !a.isRotatingHeldObject && a.activeUIElement == "" {
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
		} else if a.isRotatingHeldObject { // If rotating held object, prioritize that
			if a.firstMouse { // This firstMouse is for the rotation of held object
				a.lastMouseXForRotation, a.lastMouseYForRotation = a.window.GetCursorPos()
			}
			xoffset := float32(xpos - a.lastMouseXForRotation)
			yoffset := float32(a.lastMouseYForRotation - ypos) // Reversed Y-coordinates
			a.lastMouseXForRotation = xpos
			a.lastMouseYForRotation = ypos

			// Apply angular velocity to held object
			if a.heldObject != nil {
				// Crude rotation, needs more precise handling for real physics
				a.heldObject.AngularVelocity = mgl32.Vec3{yoffset * 0.1, xoffset * 0.1, 0} // Example
			}
		} else {
			a.firstMouse = true // Reset when mouse button is released or UI is active
		}
	})

	a.window.SetMouseButtonCallback(func(_ *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if button == glfw.MouseButtonLeft {
			if action == glfw.Press {
				a.mouseLeftPressed = true
				a.mouseLeftReleased = false // Reset released state

				// Only attempt to pick up object if mouse is grabbed AND not interacting with UI
				if a.isMouseGrabbed && a.activeUIElement == "" {
					a.tryPickObject()
				}

			} else if action == glfw.Release {
				a.mouseLeftReleased = true
				a.mouseLeftPressed = false // Reset pressed state
				a.activeUIElement = "" // Release any active UI element

				// Release held object if left click released and it was held
				if a.heldObject != nil {
					a.releaseHeldObject()
				}
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

	a.window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		if a.heldObject != nil {
			a.holdDistance = mgl32.Clamp(a.holdDistance-float32(yoff)*ScrollSensitivity, MinHoldDistance, MaxHoldDistance)
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

	// Toggle mouse grab with ESC
	if a.window.GetKey(glfw.KeyEscape) == glfw.Press {
		if a.isMouseGrabbed {
			a.isMouseGrabbed = false
			a.window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
			a.firstMouse = true // Reset mouse state when ungrabbed
			// Release held object if any
			if a.heldObject != nil {
				a.releaseHeldObject()
			}
		} else { // If not grabbed, re-grab on ESC (or click)
			a.isMouseGrabbed = true
			a.window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
			a.firstMouse = true // Reset mouse state when grabbed
		}
	}

	// Toggle E GUI with 'E' key
	currentEState := a.window.GetKey(glfw.KeyE)
	if currentEState == glfw.Press && !a.eKeyWasPressed {
		a.isEGUIVisible = !a.isEGUIVisible
		if a.isEGUIVisible {
			a.isMouseGrabbed = false
			a.window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
			if a.heldObject != nil { // Release object if E GUI is opened while holding
				a.releaseHeldObject()
			}
			log.Println("E GUI: Visible. Mouse released.")
		} else {
			a.isMouseGrabbed = true
			a.window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
			log.Println("E GUI: Hidden. Mouse grabbed.")
		}
		a.firstMouse = true // Reset mouse state when toggling E GUI
	}
	a.eKeyWasPressed = (currentEState == glfw.Press)


	// Handle 'R' key for rotating held object
	if a.heldObject != nil {
		if a.window.GetKey(glfw.KeyR) == glfw.Press {
			if !a.isRotatingHeldObject {
				a.isRotatingHeldObject = true
				// Capture current mouse position for relative rotation
				a.lastMouseXForRotation, a.lastMouseYForRotation = a.window.GetCursorPos()
			}
		} else {
			if a.isRotatingHeldObject {
				a.isRotatingHeldObject = false
				// When R is released, stop applying new angular velocity from mouse,
				// but let existing angular velocity decay naturally (or stop immediately if desired).
				// For now, we'll just stop applying new input.
				// a.heldObject.AngularVelocity = mgl32.Vec3{0,0,0} // Uncomment to stop rotation immediately
			}
		}
	}


	// Camera movement (WASD) - only if mouse is grabbed AND not rotating held object AND no UI element is active
	if a.isMouseGrabbed && !a.isRotatingHeldObject && a.activeUIElement == "" {
		currentCameraSpeed := cameraSpeed // Base speed (now float32)

		// Sprint (Shift)
		if a.window.GetKey(glfw.KeyLeftShift) == glfw.Press || a.window.GetKey(glfw.KeyRightShift) == glfw.Press {
			currentCameraSpeed *= 2.0 // Double speed
		}
		// Super Speed (Caps Lock)
		if a.window.GetKey(glfw.KeyCapsLock) == glfw.Press {
			currentCameraSpeed *= 5.0 // Five times base speed (additive with shift)
		}

		moveSpeed := currentCameraSpeed * deltaTime
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

// updateEngine handles game logic and physics.
func (a *AppCore) updateEngine(deltaTime float32) {
	// Fixed timestep physics update
	a.physicsAccumulator += deltaTime
	for a.physicsAccumulator >= PhysicsTimestep {
		a.updatePhysics(PhysicsTimestep)
		a.physicsAccumulator -= PhysicsTimestep
	}

	// Handle held object
	if a.heldObject != nil {
		// Calculate target position in front of camera
		targetPos := a.cameraPos.Add(a.cameraFront.Mul(a.holdDistance))
		a.heldObject.Position = targetPos
		a.heldObject.IsKinematic = true // Held objects are kinematic

		// Apply angular velocity to held object based on mouse input if rotating
		if a.isRotatingHeldObject {
			// Angular velocity is already being set in SetCursorPosCallback
			// We just need to integrate it here.
			a.heldObject.Rotation = a.heldObject.Rotation.Add(a.heldObject.AngularVelocity.Mul(deltaTime))
		}
	}
}

// updatePhysics updates the physics state of all objects.
func (a *AppCore) updatePhysics(dt float32) {
	for _, obj := range a.objects {
		if obj.IsKinematic {
			// If object is kinematic (e.g., held by hand or static ground),
			// clear its velocity and angular velocity so it doesn't move due to physics.
			obj.Velocity = mgl32.Vec3{0,0,0}
			obj.AngularVelocity = mgl32.Vec3{0,0,0}
			continue
		}

		// Apply gravity
		obj.Velocity[1] += Gravity * dt // Y-component for gravity

		// Update position based on velocity
		obj.Position = obj.Position.Add(obj.Velocity.Mul(dt))

		// Update rotation based on angular velocity
		obj.Rotation = obj.Rotation.Add(obj.AngularVelocity.Mul(dt))

		// Simple ground collision
		// For a box, check its lowest point relative to its local BoundingBox Min Y
		// This is a simplification; a full AABB-plane collision is more complex.
		// We're assuming the bounding box is centered around the object's origin (0,0,0)
		// and its Y extent is from Min.Y to Max.Y.
		// So, the lowest point of the object is its Position.Y + Scale.Y * BoundingBox.Min.Y
		lowestPointY := obj.Position.Y() + obj.Scale.Y() * obj.BoundingBox.Min.Y()

		if lowestPointY < GroundPlaneY {
			obj.Position[1] = GroundPlaneY - obj.Scale.Y() * obj.BoundingBox.Min.Y() // Snap to ground
			obj.Velocity[1] = 0 // Stop vertical velocity
			obj.IsGrounded = true
			// Apply some damping to horizontal velocity to simulate friction
			obj.Velocity[0] *= 0.9
			obj.Velocity[2] *= 0.9
			// Damp angular velocity as well
			obj.AngularVelocity = obj.AngularVelocity.Mul(0.9)
		} else {
			obj.IsGrounded = false
		}
	}
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
	if a.isEGUIVisible {
		a.drawEGUI() // Draw the E GUI if it's visible
	}

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

	// --- Engine Tools Panel ---
	panelX := uiPadding
	panelWidth := uiPanelWidth
	panelHeight := float32(0.0) // Will calculate dynamically

	// "Import Model" button (retained for now, but will just log a message)
	buttonX := panelX + uiPadding
	buttonY := currentY + uiPadding
	buttonWidth := panelWidth - uiPadding*2
	buttonHeight := uiButtonHeight

	if a.handleButton(buttonX, buttonY, buttonWidth, buttonHeight, "Import Model (.holym) - Not Supported") {
		log.Println("Importing .holym models is not supported in this version.")
	}
	currentY += uiButtonHeight + uiElementSpacing

	// "Spawn Box" button (from E menu request) - This will be moved to E GUI
	// if a.handleButton(panelX+uiPadding, currentY, panelWidth-uiPadding*2, uiButtonHeight, "Spawn Box") {
	// 	// Spawn a box a bit in front of the camera
	// 	spawnPos := a.cameraPos.Add(a.cameraFront.Mul(InitialHoldDistance))
	// 	a.createPrimitive("cube", spawnPos)
	// }
	// currentY += uiButtonHeight + uiElementSpacing

	a.drawTextOverlay(panelX+uiPadding, currentY+uiPadding, "Scene Objects:", mgl32.Vec4{1,1,1,1})
	currentY += uiTextHeight + uiElementSpacing

	// Object List (simplified, just selectable text)
	if len(a.objects) == 0 {
		a.drawTextOverlay(panelX+uiPadding, currentY+uiPadding, "No objects in scene.", mgl32.Vec4{0.7,0.7,0.7,1})
		currentY += uiTextHeight + uiElementSpacing
	} else {
		for _, obj := range a.objects {
			// Skip the ground plane in the list
			if obj.ID == "GroundPlane" {
				continue
			}
			label := obj.ID
			if obj == a.selectedObject {
				label += " (Selected)"
			}
			if obj == a.heldObject {
				label += " (Held)"
			}
			// For simplicity, we'll make the whole area clickable like a button
			if a.handleButton(panelX+uiPadding, currentY, panelWidth-uiPadding*2, uiButtonHeight, label) {
				a.selectedObject = obj
			}
			currentY += uiButtonHeight + uiElementSpacing
		}
	}

	panelHeight = currentY + uiPadding - uiPadding // Adjust for final padding
	a.drawRect(panelX, uiPadding, panelWidth, panelHeight, mgl32.Vec4{0.15, 0.15, 0.15, 0.8}) // Background for Engine Tools

	// --- Properties Panel for selected object ---
	if a.selectedObject != nil {
		propPanelX := float32(a.width) - uiPanelWidth - uiPadding
		propPanelY := uiPadding
		propPanelWidth := uiPanelWidth
		propPanelHeight := float32(0.0)

		currentPropY := propPanelY + uiPadding

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Properties: %s", a.selectedObject.ID), mgl32.Vec4{1,1,1,1})
		currentPropY += uiTextHeight + uiElementSpacing

		// Position Sliders
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Position X:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"pos_x", &a.selectedObject.Position[0], -20.0, 20.0)
		currentPropY += uiSliderHeight + uiElementSpacing

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Position Y:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"pos_y", &a.selectedObject.Position[1], -20.0, 20.0)
		currentPropY += uiSliderHeight + uiElementSpacing

		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Position Z:", mgl32.Vec4{1,1,1,1})
		currentPropY += uiElementSpacing
		a.handleSlider(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiSliderHeight,
			"pos_z", &a.selectedObject.Position[2], -20.0, 20.0)
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

		// Velocity (read-only)
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Velocity X: %.2f", a.selectedObject.Velocity.X()), mgl32.Vec4{1,1,1,1})
		currentPropY += uiTextHeight + uiElementSpacing
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Velocity Y: %.2f", a.selectedObject.Velocity.Y()), mgl32.Vec4{1,1,1,1})
		currentPropY += uiTextHeight + uiElementSpacing
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Velocity Z: %.2f", a.selectedObject.Velocity.Z()), mgl32.Vec4{1,1,1,1})
		currentPropY += uiTextHeight + uiElementSpacing

		// Angular Velocity (read-only)
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Ang. Vel X: %.2f", mgl32.RadToDeg(a.selectedObject.AngularVelocity.X())), mgl32.Vec4{1,1,1,1})
		currentPropY += uiTextHeight + uiElementSpacing
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Ang. Vel Y: %.2f", mgl32.RadToDeg(a.selectedObject.AngularVelocity.Y())), mgl32.Vec4{1,1,1,1})
		currentPropY += uiTextHeight + uiElementSpacing
		a.drawTextOverlay(propPanelX+uiPadding, currentPropY, fmt.Sprintf("Ang. Vel Z: %.2f", mgl32.RadToDeg(a.selectedObject.AngularVelocity.Z())), mgl32.Vec4{1,1,1,1})
		currentPropY += uiTextHeight + uiElementSpacing

		// IsKinematic toggle
		if a.selectedObject != a.heldObject { // Cannot toggle kinematic if held by hand
			if a.handleButton(propPanelX+uiPadding, currentPropY, propPanelWidth-uiPadding*2, uiButtonHeight, fmt.Sprintf("Is Kinematic: %t", a.selectedObject.IsKinematic)) {
				a.selectedObject.IsKinematic = !a.selectedObject.IsKinematic
				// If made non-kinematic, apply gravity if not already
				if !a.selectedObject.IsKinematic {
					a.selectedObject.Velocity = mgl32.Vec3{0,0,0} // Reset velocity for physics
					a.selectedObject.AngularVelocity = mgl32.Vec3{0,0,0} // Reset angular velocity
				}
			}
		} else {
			a.drawTextOverlay(propPanelX+uiPadding, currentPropY, "Is Kinematic: True (Held)", mgl32.Vec4{0.7,0.7,0.7,1})
		}
		currentPropY += uiButtonHeight + uiElementSpacing

		propPanelHeight = currentPropY - propPanelY + uiPadding
		a.drawRect(propPanelX, propPanelY, propPanelWidth, propPanelHeight, mgl32.Vec4{0.15, 0.15, 0.15, 0.8}) // Background for Properties
	}

	gl.Enable(gl.DEPTH_TEST) // Re-enable depth test for 3D scene
}

// drawEGUI renders the GUI that appears when 'E' is pressed.
func (a *AppCore) drawEGUI() {
	gl.Disable(gl.DEPTH_TEST) // Ensure UI is drawn on top
	gl.UseProgram(a.uiProgram)

	// Calculate center position for the E GUI panel
	panelWidth := float32(400) // Example width
	panelHeight := float32(300) // Example height
	panelX := (float32(a.width) - panelWidth) / 2
	panelY := (float32(a.height) - panelHeight) / 2

	a.drawRect(panelX, panelY, panelWidth, panelHeight, mgl32.Vec4{0.1, 0.1, 0.1, 0.9}) // Dark, semi-transparent background
	a.drawTextOverlay(panelX+uiPadding, panelY+uiPadding, "Spawn Menu", mgl32.Vec4{1,1,1,1})

	// Grid layout for items
	gridStartX := panelX + uiPadding
	gridStartY := panelY + uiPadding + uiTextHeight + uiElementSpacing
	itemSize := float32(80)
	itemSpacing := float32(10) // Used for spacing between grid items

	// First square: Cube preview
	cubeItemX := gridStartX
	cubeItemY := gridStartY

	// Draw the square for the cube preview
	a.drawRect(cubeItemX, cubeItemY, itemSize, itemSize, mgl32.Vec4{0.25, 0.25, 0.25, 1.0})
	a.drawTextOverlay(cubeItemX + itemSize/2 - float32(len("Cube")*3), cubeItemY + itemSize/2 - 8, "Cube", mgl32.Vec4{1,1,1,1})

	// Handle click for the cube preview
	if a.handleButton(cubeItemX, cubeItemY, itemSize, itemSize, "Spawn Cube Button") {
		spawnPos := a.cameraPos.Add(a.cameraFront.Mul(InitialHoldDistance))
		newCube := a.createPrimitive("cube", spawnPos)
		a.heldObject = newCube // Immediately grab the spawned cube
		a.isEGUIVisible = false // Close the E GUI
		a.isMouseGrabbed = true // Re-grab mouse
		a.window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		log.Println("Spawned and grabbed a cube from E GUI.")
	}

	// Example of another item: Sphere preview
	nextItemX := gridStartX + itemSize + itemSpacing // Use itemSpacing for horizontal offset
	sphereItemX := nextItemX
	sphereItemY := gridStartY

	// Draw the square for the sphere preview
	a.drawRect(sphereItemX, sphereItemY, itemSize, itemSize, mgl32.Vec4{0.25, 0.25, 0.25, 1.0})
	a.drawTextOverlay(sphereItemX + itemSize/2 - float32(len("Sphere")*3), sphereItemY + itemSize/2 - 8, "Sphere", mgl32.Vec4{1,1,1,1})

	// Handle click for the sphere preview
	if a.handleButton(sphereItemX, sphereItemY, itemSize, itemSize, "Spawn Sphere Button") {
		spawnPos := a.cameraPos.Add(a.cameraFront.Mul(InitialHoldDistance))
		newSphere := a.createPrimitive("sphere", spawnPos) // Call createPrimitive for sphere
		a.heldObject = newSphere // Immediately grab the spawned sphere
		a.isEGUIVisible = false // Close the E GUI
		a.isMouseGrabbed = true // Re-grab mouse
		a.window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		log.Println("Spawned and grabbed a sphere from E GUI.")
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
	clicked := false
	if isOver && a.mouseLeftPressed {
		buttonColor = mgl32.Vec4{0.1, 0.1, 0.1, 1.0} // Pressed color
		// If a button is pressed, it becomes the active UI element
		a.activeUIElement = label
	}

	// Only register click if mouse was released AND this element was the active one
	if isOver && a.mouseLeftReleased && a.activeUIElement == label {
		clicked = true
	}

	a.drawRect(x, y, width, height, buttonColor)
	a.drawTextOverlay(x + width/2 - float32(len(label)*3), y + height/2 - 8, label, mgl32.Vec4{1,1,1,1}) // Crude text centering

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

// --- Hand Tool / Object Picking Functions ---

// getRayFromMouse creates a ray from the camera through the mouse cursor position.
func (a *AppCore) getRayFromMouse() (origin, direction mgl32.Vec3) {
	// Screen coordinates to Normalized Device Coordinates (NDC)
	// (x, y) from (0,0) top-left to (width, height) bottom-right
	// NDC x: -1 (left) to 1 (right)
	// NDC y: 1 (top) to -1 (bottom)
	// NDC z: -1 (near) to 1 (far) (though for raycasting, we often use 0 for near, 1 for far)
	ndcX := (a.mousePosX/float32(a.width))*2.0 - 1.0
	ndcY := 1.0 - (a.mousePosY/float32(a.height))*2.0 // Y is inverted

	// Clip space coordinates
	clipCoords := mgl32.Vec4{ndcX, ndcY, -1.0, 1.0} // -1.0 for Z means near plane

	// Inverse Projection Matrix - Corrected: Use projection.Inv()
	projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(a.width)/float32(a.height), 0.1, farClippingPlane)
	invProjection := projection.Inv()

	// Eye space coordinates
	eyeCoords := invProjection.Mul4x1(clipCoords)
	eyeCoords = mgl32.Vec4{eyeCoords.X(), eyeCoords.Y(), -1.0, 0.0} // Z = -1.0, W = 0.0 for direction vector

	// Inverse View Matrix - Corrected: Use view.Inv()
	view := mgl32.LookAtV(a.cameraPos, a.cameraPos.Add(a.cameraFront), a.cameraUp)
	invView := view.Inv()

	// World space coordinates (ray direction)
	worldRay := invView.Mul4x1(eyeCoords)
	rayDirection := mgl32.Vec3{worldRay.X(), worldRay.Y(), worldRay.Z()}.Normalize()

	return a.cameraPos, rayDirection
}

// intersectRayAABB checks if a ray intersects an AABB.
// Returns true and intersection distance if it hits, false otherwise.
// Intersection algorithm based on "An Efficient and Robust Ray-Box Intersection Algorithm" by Amy Williams et al.
func intersectRayAABB(rayOrigin, rayDirection mgl32.Vec3, boxMin, boxMax mgl32.Vec3) (bool, float32) {
	tMin := float32(0.0)
	tMax := float32(math.Inf(1)) // Positive infinity

	for i := 0; i < 3; i++ { // For X, Y, Z axes
		if math.Abs(float64(rayDirection[i])) < 1e-6 { // Ray is parallel to slab
			if rayOrigin[i] < boxMin[i] || rayOrigin[i] > boxMax[i] {
				return false, 0 // No hit
			}
		} else {
			t1 := (boxMin[i] - rayOrigin[i]) / rayDirection[i]
			t2 := (boxMax[i] - rayOrigin[i]) / rayDirection[i]

			if t1 > t2 {
				t1, t2 = t2, t1 // Swap to ensure t1 is always smaller
			}

			tMin = float32(math.Max(float64(tMin), float64(t1)))
			tMax = float32(math.Min(float64(tMax), float64(t2)))

			if tMin > tMax {
				return false, 0 // No hit
			}
		}
	}
	return true, tMin
}

// tryPickObject attempts to pick up an object using a raycast.
func (a *AppCore) tryPickObject() {
	rayOrigin, rayDirection := a.getRayFromMouse()

	closestHit := float32(PickupRange + 1.0) // Initialize with a value outside pickup range
	var hitObject *GameObject = nil

	for _, obj := range a.objects {
		// Skip ground plane and currently held object
		if obj.ID == "GroundPlane" || obj == a.heldObject {
			continue
		}

		// Transform object's local bounding box to world space
		// Corrected: Manual component-wise multiplication instead of MulV
		scaledMin := obj.Position.Add(mgl32.Vec3{
			obj.BoundingBox.Min.X() * obj.Scale.X(),
			obj.BoundingBox.Min.Y() * obj.Scale.Y(),
			obj.BoundingBox.Min.Z() * obj.Scale.Z(),
		})
		scaledMax := obj.Position.Add(mgl32.Vec3{
			obj.BoundingBox.Max.X() * obj.Scale.X(),
			obj.BoundingBox.Max.Y() * obj.Scale.Y(),
			obj.BoundingBox.Max.Z() * obj.Scale.Z(),
		})

		if hit, dist := intersectRayAABB(rayOrigin, rayDirection, scaledMin, scaledMax); hit {
			if dist < PickupRange && dist < closestHit {
				closestHit = dist
				hitObject = obj
			}
		}
	}

	if hitObject != nil {
		a.heldObject = hitObject
		a.heldObject.IsKinematic = true // Disable physics while held
		a.heldObject.Velocity = mgl32.Vec3{0,0,0} // Stop any current motion
		a.heldObject.AngularVelocity = mgl32.Vec3{0,0,0} // Stop any current rotation
		log.Printf("Picked up object: %s", a.heldObject.ID)
	}
}

// releaseHeldObject releases the currently held object.
func (a *AppCore) releaseHeldObject() {
	if a.heldObject != nil {
		a.heldObject.IsKinematic = false // Re-enable physics

		// Apply a throw force based on camera direction
		throwDirection := a.cameraFront.Normalize()
		a.heldObject.Velocity = throwDirection.Mul(ThrowForceMagnitude)

		// If it was rotating, maintain angular velocity, otherwise clear it
		if !a.isRotatingHeldObject {
			a.heldObject.AngularVelocity = mgl32.Vec3{0,0,0}
		}

		log.Printf("Released object: %s", a.heldObject.ID)
		a.heldObject = nil
		a.isRotatingHeldObject = false // Ensure rotation state is reset
	}
}


// --- Model/Primitive Creation Functions ---

// newTexture creates an OpenGL texture from an image.
func newTexture(img image.Image) (uint32, error) {
	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)

	// Set texture wrapping and filtering options
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR) // Use mipmaps
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	rgba := image.NewRGBA(img.Bounds())
	// Ensure that the image is copied into an RGBA format that OpenGL expects
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)

	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(rgba.Rect.Size().X), int32(rgba.Rect.Size().Y), 0,
		gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(rgba.Pix))
	gl.GenerateMipmap(gl.TEXTURE_2D)

	gl.BindTexture(gl.TEXTURE_2D, 0) // Unbind texture

	return texture, nil
}

// createGameObject initializes OpenGL buffers for a new GameObject and adds it to the scene.
func (a *AppCore) createGameObject(id string, vertices []float32, indices []uint32, hasTexture bool, texturePath string, initialPos mgl32.Vec3, mass float32, boundingBox BoundingBox) *GameObject {
	newObj := &GameObject{
		ID:           id,
		Vertices:     vertices,
		Indices:      indices,
		IndicesCount: int32(len(indices)),
		Position:     initialPos,
		Rotation:     mgl32.Vec3{0, 0, 0}, // Initial rotation
		Scale:        mgl32.Vec3{1, 1, 1}, // Initial scale
		HasTexture:   hasTexture,
		TexturePath:  texturePath,
		Velocity:     mgl32.Vec3{0, 0, 0},
		AngularVelocity: mgl32.Vec3{0,0,0},
		IsKinematic:  false, // Start as dynamic unless explicitly set
		IsGrounded:   false,
		Mass:         mass,
		BoundingBox:  boundingBox,
	}

	// Load texture if path is provided
	if newObj.HasTexture && newObj.TexturePath != "" {
		texID, err := newTextureFromFile(newObj.TexturePath) // Use helper to load from file
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

// newTextureFromFile loads an image from a file and creates an OpenGL texture.
func newTextureFromFile(imgPath string) (uint32, error) {
	file, err := os.Open(imgPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open texture file %s: %w", imgPath, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return 0, fmt.Errorf("failed to decode texture image %s: %w", imgPath, err)
	}
	return newTexture(img)
}

// loadHolymModel is now a placeholder as per user request.
func (a *AppCore) loadHolymModel(filePath string) error {
	log.Printf("Loading .holym models is currently disabled. Attempted to load: %s", filePath)
	// For demonstration, we could spawn a default cube instead of loading the model
	// a.createPrimitive("cube", a.cameraPos.Add(a.cameraFront.Mul(InitialHoldDistance)))
	return nil // Return nil error to indicate it "handled" the request gracefully
}

// createPrimitive generates a new primitive shape and adds it to the scene.
// It now returns the created GameObject.
func (a *AppCore) createPrimitive(shapeType string, initialPos mgl32.Vec3) *GameObject {
	var vertices []float32
	var indices []uint32
	var id string
	var bbox BoundingBox
	var mass float32 = 1.0 // Default mass for primitives

	switch shapeType {
	case "cube":
		id = fmt.Sprintf("Cube_%d", a.nextObjectID)
		vertices, indices = generateCubeData()
		bbox = BoundingBox{Min: mgl32.Vec3{-0.5, -0.5, -0.5}, Max: mgl32.Vec3{0.5, 0.5, 0.5}} // Unit cube
	case "plane":
		id = fmt.Sprintf("Plane_%d", a.nextObjectID)
		vertices, indices = generatePlaneData()
		// For a plane, the bounding box typically has zero height on the plane axis
		bbox = BoundingBox{Min: mgl32.Vec3{-0.5, 0.0, -0.5}, Max: mgl32.Vec3{0.5, 0.0, 0.5}} // Unit plane on Y=0
		mass = 0.0 // Planes are static/immovable
	case "sphere": // New case for sphere
		id = fmt.Sprintf("Sphere_%d", a.nextObjectID)
		// Placeholder for sphere data generation.
		// In a real engine, you'd have a generateSphereData() function.
		// For now, we'll use cube data as a visual stand-in to avoid a crash,
		// but log a warning.
		log.Println("Warning: Sphere generation not implemented. Using cube data as placeholder.")
		vertices, indices = generateCubeData() // Placeholder: use cube data
		bbox = BoundingBox{Min: mgl32.Vec3{-0.5, -0.5, -0.5}, Max: mgl32.Vec3{0.5, 0.5, 0.5}} // Placeholder: cube bbox
	default:
		log.Printf("Unsupported primitive type: %s", shapeType)
		return nil // Return nil if unsupported
	}

	newObj := a.createGameObject(id, vertices, indices, false, "", initialPos, mass, bbox)
	if shapeType == "plane" {
		newObj.IsKinematic = true // Ground plane should be kinematic
	}
	a.selectedObject = newObj
	log.Printf("Created primitive: %s", id)
	return newObj // Return the created object
}

// createGroundPlane creates a large, static ground plane.
func (a *AppCore) createGroundPlane() {
	vertices, indices := generatePlaneData()
	ground := a.createGameObject(
		"GroundPlane",
		vertices,
		indices,
		false, // No texture for now
		"",
		mgl32.Vec3{0, GroundPlaneY, 0}, // Position at the ground plane Y
		0.0, // Infinite mass, doesn't move
		BoundingBox{Min: mgl32.Vec3{-0.5, 0.0, -0.5}, Max: mgl32.Vec3{0.5, 0.0, 0.5}}, // Unit plane bbox
	)
	ground.Scale = mgl32.Vec3{100, 1, 100} // Make it large
	ground.IsKinematic = true // It's a static part of the environment
	log.Println("Created ground plane.")
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

	log.Println("Holy Engine Base initialized. Starting main loop...")
	log.Println("Controls:")
	log.Println("  WASD: Move camera")
	log.Println("  Mouse: Look around (when grabbed)")
	log.Println("  ESC: Toggle mouse grab / Exit")
	log.Println("  E: Toggle Spawn Menu (releases mouse grab)")
	log.Println("  Left-click (when grabbed): Pick up/Throw object")
	log.Println("  R (hold) + Mouse (when holding object): Rotate held object")
	log.Println("  Scroll Wheel (when holding object): Adjust hold distance")
	log.Println("  Shift: Sprint")
	log.Println("  Caps Lock: Super Speed")
	log.Println("  Use UI panels to Spawn Boxes and Transform Selected Objects.")

	// Main Engine Loop
	for !app.shouldClose() {
		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(app.lastFrameTime).Seconds())
		app.lastFrameTime = currentTime

		// Process input (handles custom UI interaction, camera, and object picking)
		app.processInput(deltaTime)

		// Update engine logic (physics, held objects)
		app.updateEngine(deltaTime)

		// Render the scene (3D objects + Custom UI)
		app.renderScene()
		app.updateAndDisplayFPS()

		// Reset mouse released state after processing for this frame
		app.mouseLeftReleased = false
	}

	log.Println("Holy Engine Base shutting down.")
}
