package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	objparser "github.com/go-gl/mathgl/obj" // Changed: Using github.com/go-gl/mathgl/obj for OBJ parsing
)

// Constants for window dimensions
const (
	screenWidth  = 800
	screenHeight = 600
	windowTitle  = "Go OpenGL Spinning 3D Model (GLFW)"
	// Changed default from GLB file to OBJ model base directory
	defaultModelBaseDir = "default/"
)

// Camera and Input Constants
const (
	cameraSpeed      = 250.0 // Units per second
	mouseSensitivity = 0.1   // Degrees per pixel
	farClippingPlane = 5000.0 // Increased for distant objects
)

// AppCore struct encapsulates the low-level graphics and windowing components.
type AppCore struct {
	window *glfw.Window

	// OpenGL program and buffers for the model
	program      uint32
	vao          uint32
	vbo          uint32
	ebo          uint32
	indicesCount int32
	textureID    uint32

	// Uniform locations
	modelUniform      int32
	viewUniform       int32
	projectionUniform int32
	textureUniform    int32

	// Window dimensions
	width, height int
	title         string

	// Internal state for main loop
	running bool

	// Model data (loaded from OBJ)
	vertices []float32
	indices  []uint32

	// Game state for animation
	totalRotationX float32
	totalRotationY float32
	lastFrameTime  time.Time

	// FPS Counter state
	fpsFrames      int
	fpsLastUpdateTime time.Time

	// VSync Control state
	vsyncEnabled    bool
	vKeyWasPressed bool

	// Camera Control state
	cameraPos   mgl32.Vec3
	cameraFront mgl32.Vec3
	cameraUp    mgl32.Vec3
	zoomLevel   float32

	// Mouse Look State
	firstMouse bool
	mouseLastX float64
	mouseLastY float64
	yaw        float32
	pitch      float32
	rightMouseButtonPressed bool

	// Rotation Toggle State
	rotationEnabled bool
	rKeyWasPressed  bool

	// Custom Model Loading State
	gKeyWasPressed bool
}

// Global instance of AppCore
var app *AppCore

// initApp initializes GLFW, OpenGL context, and prepares the core rendering data.
func initApp() error {
	runtime.LockOSThread()

	app = &AppCore{
		width:   screenWidth,
		height:  screenHeight,
		title:   windowTitle,
		running: true,
		cameraPos:   mgl32.Vec3{0, 0, 5.0},
		cameraFront: mgl32.Vec3{0, 0, -1},
		cameraUp:    mgl32.Vec3{0, 1, 0},
		zoomLevel:   5.0,
		firstMouse:  true,
		yaw:         -90.0,
		pitch:       0.0,
		rotationEnabled: true,
	}

	// Load the 3D model from OBJ files, starting with the default directory
	if err := app.loadAndSetupModel(defaultModelBaseDir); err != nil {
		return fmt.Errorf("failed to load default OBJ model from %s: %w", defaultModelBaseDir, err)
	}

	if err := app.initializeWindow(); err != nil {
		return fmt.Errorf("window initialization failed: %w", err)
	}

	if err := app.initializeOpenGL(); err != nil {
		return fmt.Errorf("OpenGL initialization failed: %w", err)
	}

	if err := app.setupShadersAndUniforms(); err != nil {
		return fmt.Errorf("shader setup failed: %w", err)
	}

	app.setupCameraAndProjection()

	app.lastFrameTime = time.Now()
	app.fpsLastUpdateTime = time.Now()
	app.fpsFrames = 0

	return nil
}

// ObjVertex struct for creating unique vertex combinations from OBJ data
type ObjVertex struct {
	Pos      mgl32.Vec3
	TexCoord mgl32.Vec2
}

// loadAndSetupModel loads an OBJ model and sets up its OpenGL buffers and texture.
// It takes the base directory of the model (e.g., "my_model_folder/").
// Assumes OBJ is in baseDir/source/ and textures are in baseDir/textures/
func (a *AppCore) loadAndSetupModel(baseDir string) error {
	// Clean up previous buffers if any
	if a.vao != 0 {
		gl.DeleteVertexArrays(1, &a.vao)
		gl.DeleteBuffers(1, &a.vbo)
		gl.DeleteBuffers(1, &a.ebo)
		if a.textureID != 0 {
			gl.DeleteTextures(1, &a.textureID)
			a.textureID = 0 // Reset texture ID
		}
	}

	// Determine the main OBJ file name (e.g., "default.obj" from "default/" folder)
	modelName := strings.TrimSuffix(filepath.Base(baseDir), string(os.PathSeparator))
	if modelName == "" { // Handle cases like "." or "/"
		modelName = "default" // Fallback name
	}

	objFilePath := filepath.Join(baseDir, "source", modelName+".obj")
	// The objparser.Parse function from go-gl/mathgl/obj expects the MTL path
	// to be relative to the OBJ file, or absolute.
	// We'll pass the directory containing the OBJ file as the base for MTL lookup.
	mtlDir := filepath.Join(baseDir, "source")

	objFile, err := os.Open(objFilePath)
	if err != nil {
		return fmt.Errorf("could not open OBJ file %s: %w", objFilePath, err)
	}
	defer objFile.Close()

	// Use go-gl/mathgl/obj.Parse to parse the OBJ file and its associated MTL
	objModel, err := objparser.Parse(objFile, mtlDir)
	if err != nil {
		return fmt.Errorf("failed to parse OBJ file %s: %w", objFilePath, err)
	}

	var vertices []float32
	var indices []uint32
	vertexMap := make(map[ObjVertex]uint32) // Map to store unique vertex combinations
	currentIdx := uint32(0)

	// Iterate through faces to build interleaved data
	for _, face := range objModel.Faces {
		if len(face.Vertices) != 3 { // We are only handling triangles for simplicity
			log.Printf("Warning: Face with %d vertices encountered, skipping (only triangles supported).", len(face.Vertices))
			continue
		}
		for i := 0; i < 3; i++ { // Iterate through the 3 vertices of the triangle
			vertexIdx := face.Vertices[i]
			texCoordIdx := face.TexCoords[i] // go-gl/mathgl/obj separates these indices

			// go-gl/mathgl/obj uses 0-based indices directly
			pos := objModel.Positions[vertexIdx]
			uv := mgl32.Vec2{0, 0} // Default UV if not found

			if texCoordIdx >= 0 && texCoordIdx < len(objModel.TexCoords) {
				rawUV := objModel.TexCoords[texCoordIdx]
				uv = mgl32.Vec2{rawUV.X(), rawUV.Y()} // TexCoords are mgl32.Vec2 in this library
			} else {
				log.Printf("Warning: Missing or invalid texture coordinate index for vertex in face. Defaulting to (0,0).")
			}

			v := ObjVertex{
				Pos:      pos, // Positions are already mgl32.Vec3
				TexCoord: uv,
			}

			if idx, ok := vertexMap[v]; ok {
				indices = append(indices, idx)
			} else {
				vertexMap[v] = currentIdx
				indices = append(indices, currentIdx)
				vertices = append(vertices, v.Pos.X(), v.Pos.Y(), v.Pos.Z())
				vertices = append(vertices, v.TexCoord.X(), v.TexCoord.Y())
				currentIdx++
			}
		}
	}

	a.vertices = vertices
	a.indices = indices
	a.indicesCount = int32(len(a.indices))

	// --- Load Texture ---
	a.textureID = 0 // Default to 0 if no texture is found/loaded
	if len(objModel.Materials) > 0 {
		// Iterate through materials to find a diffuse texture map
		for _, mtl := range objModel.Materials {
			if mtl.MapKd != "" { // MapKd is the diffuse texture map
				// Construct the full path to the texture file
				texturePath := filepath.Join(baseDir, "textures", mtl.MapKd)
				imgFile, err := os.Open(texturePath)
				if err != nil {
					log.Printf("Warning: Could not open texture file %s for material %s: %v", texturePath, mtl.Name, err)
					continue // Try next material
				}
				defer imgFile.Close()

				img, _, err := image.Decode(imgFile)
				if err != nil {
					log.Printf("Warning: Failed to decode texture image %s for material %s: %v", texturePath, mtl.Name, err)
					continue
				}

				a.textureID, err = newTexture(img)
				if err != nil {
					log.Printf("Warning: Failed to create OpenGL texture from %s: %v", texturePath, err)
				} else {
					log.Printf("Texture '%s' loaded successfully.", texturePath)
					break // Texture loaded, stop looking
				}
			}
		}
		if a.textureID == 0 {
			log.Println("Warning: No diffuse texture loaded for the model.")
		}
	} else {
		log.Println("Warning: No materials found in OBJ model.")
	}


	// Setup OpenGL buffers
	if a.vao == 0 {
		gl.GenVertexArrays(1, &a.vao)
		gl.GenBuffers(1, &a.vbo)
		gl.GenBuffers(1, &a.ebo)
	}

	gl.BindVertexArray(a.vao)

	gl.BindBuffer(gl.ARRAY_BUFFER, a.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(a.vertices)*4, gl.Ptr(a.vertices), gl.STATIC_DRAW)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, a.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(a.indices)*4, gl.Ptr(a.indices), gl.STATIC_DRAW)

	// Position attribute (layout location 0, 3 floats)
	// Vertex stride is 5*4 bytes (3 pos + 2 texcoord)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 5*4, gl.Ptr(nil))
	gl.EnableVertexAttribArray(0)

	// Texture coordinate attribute (layout location 1, 2 floats, offset after 3 positions)
	gl.VertexAttribPointer(1, 2, gl.FLOAT, false, 5*4, gl.PtrOffset(3*4))
	gl.EnableVertexAttribArray(1)

	gl.BindVertexArray(0) // Unbind VAO

	log.Printf("Loaded %d unique vertices and %d indices from %s", currentIdx, len(a.indices), objFilePath)
	return nil
}

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

	a.vsyncEnabled = true
	glfw.SwapInterval(1)
	log.Println("VSync ON (default)")

	a.window.SetFramebufferSizeCallback(func(_ *glfw.Window, width, height int) {
		a.width = width
		a.height = height
		gl.Viewport(0, 0, int32(width), int32(height))
		projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(a.width)/float32(a.height), 0.1, farClippingPlane)
		gl.UniformMatrix4fv(a.projectionUniform, 1, false, &projection[0])
	})

	a.window.SetScrollCallback(func(_ *glfw.Window, xoff, yoff float64) {
		zoomSensitivity := float32(0.5)
		a.zoomLevel -= float32(yoff) * zoomSensitivity
		a.updateCameraPosition()
	})

	a.window.SetCursorPosCallback(func(_ *glfw.Window, xpos, ypos float64) {
		if !a.rotationEnabled && a.window.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press {
			if a.firstMouse {
				a.mouseLastX = xpos
				a.mouseLastY = ypos
				a.firstMouse = false
			}

			xoffset := float32(xpos - a.mouseLastX)
			yoffset := float32(a.mouseLastY - ypos)
			a.mouseLastX = xpos
			a.mouseLastY = ypos

			xoffset *= mouseSensitivity
			yoffset *= mouseSensitivity

			a.totalRotationY += mgl32.DegToRad(xoffset)
			a.totalRotationX += mgl32.DegToRad(yoffset)
		} else if a.window.GetMouseButton(glfw.MouseButtonRight) == glfw.Press {
			if !a.rightMouseButtonPressed {
				a.mouseLastX = xpos
				a.mouseLastY = ypos
				a.rightMouseButtonPressed = true
			}

			xoffset := float32(xpos - a.mouseLastX)
			yoffset := float32(a.mouseLastY - ypos)
			a.mouseLastX = xpos
			a.mouseLastY = ypos

			xoffset *= mouseSensitivity
			yoffset *= mouseSensitivity

			a.yaw += xoffset
			a.pitch += yoffset

			if a.pitch > 89.0 {
				a.pitch = 89.0
			}
			if a.pitch < -89.0 {
				a.pitch = -89.0
			}

			a.updateCameraPosition()
		} else {
			a.firstMouse = true
			a.rightMouseButtonPressed = false
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

// setupShadersAndUniforms compiles shaders, links the program, and gets uniform locations.
func (a *AppCore) setupShadersAndUniforms() error {
	vertexShaderSource := `
		#version 410 core
		layout (location = 0) in vec3 aPos;
		layout (location = 1) in vec2 aTexCoord;

		out vec2 TexCoord;

		uniform mat4 model;
		uniform mat4 view;
		uniform mat4 projection;

		void main() {
			gl_Position = projection * view * model * vec4(aPos, 1.0);
			TexCoord = aTexCoord;
		}
	` + "\x00"

	fragmentShaderSource := `
		#version 410 core
		in vec2 TexCoord;
		out vec4 FragColor;

		uniform sampler2D ourTexture;

		void main() {
			FragColor = texture(ourTexture, TexCoord);
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
	a.textureUniform = gl.GetUniformLocation(a.program, gl.Str("ourTexture\x00"))

	return nil
}

// setupCameraAndProjection sets up initial view and projection matrices.
func (a *AppCore) setupCameraAndProjection() {
	a.updateCameraPosition()

	projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(a.width)/float32(a.height), 0.1, farClippingPlane)
	gl.UniformMatrix4fv(a.projectionUniform, 1, false, &projection[0])
}

// updateCameraPosition recalculates and updates the view matrix based on the current camera state.
func (a *AppCore) updateCameraPosition() {
	yawRad := mgl32.DegToRad(a.yaw)
	pitchRad := mgl32.DegToRad(a.pitch)

	frontX := float32(math.Cos(float64(yawRad)) * math.Cos(float64(pitchRad)))
	frontY := float32(math.Sin(float64(pitchRad)))
	frontZ := float32(math.Sin(float64(yawRad)) * math.Cos(float64(pitchRad)))
	a.cameraFront = mgl32.Vec3{frontX, frontY, frontZ}.Normalize()

	view := mgl32.LookAtV(a.cameraPos, a.cameraPos.Add(a.cameraFront), a.cameraUp)
	gl.UniformMatrix4fv(a.viewUniform, 1, false, &view[0])
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
			glfw.SwapInterval(1)
			log.Println("VSync: ON (FPS capped)")
		} else {
			glfw.SwapInterval(0)
			log.Println("VSync: OFF (FPS uncapped)")
		}
	}
	a.vKeyWasPressed = (currentVState == glfw.Press)

	// R key to toggle automatic rotation / manual mouse rotation
	currentRState := a.window.GetKey(glfw.KeyR)
	if currentRState == glfw.Press && !a.rKeyWasPressed {
		a.rotationEnabled = !a.rotationEnabled
		if a.rotationEnabled {
			log.Println("Automatic Rotation: ON")
		} else {
			log.Println("Automatic Rotation: OFF (Manual mouse rotation enabled)")
			a.firstMouse = true
		}
	}
	a.rKeyWasPressed = (currentRState == glfw.Press)

	// G key to load custom model from folder
	currentGState := a.window.GetKey(glfw.KeyG)
	if currentGState == glfw.Press && !a.gKeyWasPressed {
		log.Print("Enter path to model's base directory (e.g., my_model_folder/): ")
		reader := bufio.NewReader(os.Stdin)
		inputPath, _ := reader.ReadString('\n')
		inputPath = strings.TrimSpace(inputPath)

		if inputPath != "" {
			// Ensure inputPath ends with a slash for consistent directory handling
			if !strings.HasSuffix(inputPath, string(os.PathSeparator)) {
				inputPath += string(os.PathSeparator)
			}
			log.Printf("Attempting to load custom model from directory: %s", inputPath)
			if err := a.loadAndSetupModel(inputPath); err != nil {
				log.Printf("Error loading custom model from %s: %v", inputPath, err)
			} else {
				log.Printf("Successfully loaded model from %s", inputPath)
				// Reset model rotation when new model is loaded
				a.totalRotationX = 0
				a.totalRotationY = 0
			}
		} else {
			log.Println("No path entered. Keeping current model.")
		}
	}
	a.gKeyWasPressed = (currentGState == glfw.Press)

	// WASD camera movement
	cameraMoveSpeed := cameraSpeed * float32(time.Since(app.lastFrameTime).Seconds())
	if a.window.GetKey(glfw.KeyW) == glfw.Press {
		a.cameraPos = a.cameraPos.Add(a.cameraFront.Mul(cameraMoveSpeed))
	}
	if a.window.GetKey(glfw.KeyS) == glfw.Press {
		a.cameraPos = a.cameraPos.Sub(a.cameraFront.Mul(cameraMoveSpeed))
	}
	if a.window.GetKey(glfw.KeyA) == glfw.Press {
		right := a.cameraFront.Cross(a.cameraUp).Normalize()
		a.cameraPos = a.cameraPos.Sub(right.Mul(cameraMoveSpeed))
	}
	if a.window.GetKey(glfw.KeyD) == glfw.Press {
		right := a.cameraFront.Cross(a.cameraUp).Normalize()
		a.cameraPos = a.cameraPos.Add(right.Mul(cameraMoveSpeed))
	}

	a.updateCameraPosition()
}

// updateScene updates the game state (e.g., model rotation).
func (a *AppCore) updateScene(deltaTime float32) {
	if a.rotationEnabled {
		a.totalRotationY += deltaTime * mgl32.DegToRad(50.0)
		a.totalRotationX += deltaTime * mgl32.DegToRad(25.0)
	}
}

// renderScene clears buffers, draws the model, and swaps buffers.
func (a *AppCore) renderScene() {
	gl.ClearColor(0.2, 0.3, 0.3, 1.0)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, a.textureID)
	gl.Uniform1i(a.textureUniform, 0)

	model := mgl32.Ident4()
	model = model.Mul4(mgl32.HomogRotate3DY(a.totalRotationY))
	model = model.Mul4(mgl32.HomogRotate3DX(a.totalRotationX))

	a.drawModel(model)
	a.window.SwapBuffers()
}

// drawModel draws the loaded 3D model with the given model matrix.
func (a *AppCore) drawModel(modelMatrix mgl32.Mat4) {
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
		a.window.SetTitle(fmt.Sprintf("%s | FPS: %.2f", a.title, fps))
		a.fpsFrames = 0
		a.fpsLastUpdateTime = time.Now()
	}
}

// shutdownApp cleans up core OpenGL and GLFW resources.
func shutdownApp() {
	if app == nil {
		return
	}
	gl.DeleteVertexArrays(1, &app.vao)
	gl.DeleteBuffers(1, &app.vbo)
	gl.DeleteBuffers(1, &app.ebo)
	gl.DeleteProgram(app.program)
	if app.textureID != 0 {
		gl.DeleteTextures(1, &app.textureID)
	}

	if app.window != nil {
		app.window.Destroy()
	}
	glfw.Terminate()
}

// --- Helper functions for shader compilation (unchanged) ---

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

func glShaderSource(shader uint32, source string) {
	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
}

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

func (a *AppCore) shouldClose() bool {
	return a.window.ShouldClose() || !a.running
}

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer shutdownApp()

	if err := initApp(); err != nil {
		log.Fatalf("Application initialization failed: %v", err)
	}

	log.Println("Engine initialized. Starting main loop...")

	for !app.shouldClose() {
		app.processInput()

		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(app.lastFrameTime).Seconds())
		app.lastFrameTime = currentTime

		app.updateScene(deltaTime)
		app.renderScene()
		app.updateAndDisplayFPS()
	}

	log.Println("Engine shutting down.")
}
