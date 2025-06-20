# Go Shader Translator
`goshadertranslator `is a Go library that provides an easy way to translate graphics shaders between different formats. It leverages a WebAssembly (WASM) build of Google's ANGLE (Almost Native Graphics Layer Engine) library, a robust shader translator used by major web browsers to implement WebGL.

The library uses the [wazero](https://wazero.io/) runtime to execute the embedded ANGLE WASM module, allowing you to translate WebGL, OpenGL ES 2.0, and OpenGL ES 3.0 shaders into various desktop GLSL versions.

This is particularly useful for projects that need to run shaders from sources like [Shadertoy](https://www.shadertoy.com/) on native desktop applications using OpenGL, as demonstrated in the example.

## Features
* Versatile Shader Translation: Convert between multiple shader specifications.
  * Input Specs: `WebGL`, `WebGL2`, `WebGL3`, `GLES2`, `GLES3`, and more.
  * Output Formats: Desktop `GLSL` from version `130` to `450`, and `ESSL`.
* WASM-Powered: Uses a compiled WASM module of the ANGLE shader translator for high-fidelity translations.
* Self-Contained: The ANGLE WASM binary is embedded directly into the library, requiring no external dependencies for your project.
* Detailed Shader Info: Returns the translated shader code along with metadata about active uniforms, attributes, and other variables.
* Handles Name Mangling: ANGLE renames shader uniforms during translation. The library provides the original and the new "mapped" names, so you can correctly locate them in your host application.

## How it works
The library works by communicating with the embedded ANGLE WASM module over a simple JSON-RPC interface.

1. A `ShaderTranslator` instance is created, which initializes the `wazero` runtime and loads the ANGLE WASM module.
2. You call `TranslateShader()` with your shader code and desired input/output formats.
3. The library marshals this request into a JSON object and calls the `invoke` function exported by the WASM module.
4. The ANGLE module translates the shader and returns a JSON response containing the translated code and variable information.
5. The library parses this response into a simple `Shader` struct for you to use.

## Installation
```bash
go get github.com/richinsley/goshadertranslator
```

## Usage and Example
The following is a complete example of how to use `goshadertranslator` with `raylib-go` to run a shader from Shadertoy.

### Key Concepts
1. Shader Combination: A full fragment shader is constructed by combining a preamble (defining uniforms like `iResolution`, `iTime`, etc.), the shader code from Shadertoy, and a `main` function to call Shadertoy's `mainImage` function.
2. Translation: The complete shader string is passed to `translator.TranslateShader`, specifying the input as `ShaderSpecWebGL2` and the output as `OutputFormatGLSL330` for compatibility with modern desktop OpenGL.
3. Uniform Name Mangling: ANGLE changes uniform names (e.g., `iResolution` becomes `_uiResolution`). The returned Shader struct contains a `Variables` map that provides the `MappedName` for each original uniform.

### Example code (`main.go`)
```go
package main

import (
	"context"
	"fmt"
	"log"
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	gst "github.com/richinsley/goshadertranslator"
)

// --- Shadertoy Example ---
// The actual shader passed into the translator is the preamble + shadertoy shader + main function
// getPreamble returns the equivalent of the Shadertoy preamble for the fragment shader in WebGL.
func getPreamble() string {
	fragmentShader := `#version 300 es
precision highp float;
precision highp int;
precision mediump sampler3D;

uniform vec3 iResolution;
uniform float iTime;
uniform vec4 iMouse;
// Add other iChannel uniforms if needed, e.g.:
// uniform sampler2D iChannel0;
// uniform vec3 iChannelResolution[4];

in vec2 frag_coord_uv; // UV coordinates from vertex shader [0, 1]
out vec4 fragColor;    // Output color

// tanh is problematic for OpenGL implementations, so we define a fast_tanh function
// this fixes issues with many shadertoy shaders that use tanh
// ==== fast_tanh |error| < 6e-4 for |x| ≤ 4, no exp() ========
#define FAST_TANH_BODY(x)  ( (x) * (27.0 + (x)*(x)) / (27.0 + 9.0*(x)*(x)) )

float fast_tanh(float x) { return FAST_TANH_BODY(x); }
vec2  fast_tanh(vec2  x) { return FAST_TANH_BODY(x); }
vec3  fast_tanh(vec3  x) { return FAST_TANH_BODY(x); }
vec4  fast_tanh(vec4  x) { return FAST_TANH_BODY(x); }

// --------- shadow the builtin tanh from this line downward
#define tanh fast_tanh
`
	return fragmentShader
}

// getMain returns the equivalent of the Shadertoy main function for the fragment shader in WebGL.
func getMain() string {
	fragmentShader := `
void main( void )
{
	fragColor = vec4(1.0,1.0,1.0,1.0);
	vec4 color = vec4(1e20);
	vec2 uv = gl_FragCoord.xy;// * 0.5;
	mainImage( color, uv );
	if(fragColor.x<0.0) color=vec4(1.0,0.0,0.0,1.0);
	if(fragColor.y<0.0) color=vec4(0.0,1.0,0.0,1.0);
	if(fragColor.z<0.0) color=vec4(0.0,0.0,1.0,1.0);
	if(fragColor.w<0.0) color=vec4(1.0,1.0,0.0,1.0);
	fragColor = vec4(color.xyz,1.0);
}
`
	return fragmentShader
}

// GetFragmentShader combines the preamble, shader code, and main function
func GetFragmentShader(shadercode string) string {

	fragmentShader := getPreamble()
	fragmentShader += shadercode
	fragmentShader += getMain()

	return fragmentShader
}

// in main.go
func runShadertoyExample() {
	const screenWidth = 1280
	const screenHeight = 720

	// Initialize the translator
	ctx := context.Background()
	translator, err := gst.NewShaderTranslator(ctx)
	if err != nil {
		log.Fatalf("Failed to create shader translator: %v", err)
	}
	defer translator.Close()

    // Singularity by Xor
	// from https://www.shadertoy.com/view/3csSWB
	shadercode := `
/*
    "Singularity" by @XorDev

    A whirling blackhole.
    Feel free to code golf!
    
    FabriceNeyret2: -19
    dean_the_coder: -12
    iq: -4
*/
void mainImage(out vec4 O, vec2 F)
{
    //Iterator and attenuation (distance-squared)
    float i = .2, a;
    //Resolution for scaling and centering
    vec2 r = iResolution.xy,
         //Centered ratio-corrected coordinates
         p = ( F+F - r ) / r.y / .7,
         //Diagonal vector for skewing
         d = vec2(-1,1),
         //Blackhole center
         b = p - i*d,
         //Rotate and apply perspective
         c = p * mat2(1, 1, d/(.1 + i/dot(b,b))),
         //Rotate into spiraling coordinates
         v = c * mat2(cos(.5*log(a=dot(c,c)) + iTime*i + vec4(0,33,11,0)))/i,
         //Waves cumulative total for coloring
         w;
    
    //Loop through waves
    for(; i++<9.; w += 1.+sin(v) )
        //Distort coordinates
        v += .7* sin(v.yx*i+iTime) / i + .5;
    //Acretion disk radius
    i = length( sin(v/.3)*.4 + c*(3.+d) );
    //Red/blue gradient
    O = 1. - exp( -exp( c.x * vec4(.6,-.4,-1,0) )
                   //Wave coloring
                   /  w.xyyx
                   //Acretion disk brightness
                   / ( 2. + i*i/4. - i )
                   //Center darkness
                   / ( .5 + 1. / a )
                   //Rim highlight
                   / ( .03 + abs( length(p)-.7 ) )
             );
    }
`

	fmt.Println("--- Translating Fragment Shader ---")
	fsShader, err := translator.TranslateShader(GetFragmentShader(shadercode), "fragment", gst.ShaderSpecWebGL2, gst.OutputFormatGLSL330)
	if err != nil {
		log.Fatalf("%v", err)
	} else {
		fmt.Println("Fragment shader translation result:", fsShader.Code)
	}

	// Initialize Raylib and Load Shader ---
	rl.SetConfigFlags(rl.FlagWindowResizable)
	rl.InitWindow(screenWidth, screenHeight, "ANGLE + wazero + Raylib Shadertoy")
	defer rl.CloseWindow()

	// we don't need a vertex shader for libray fullscreen quad rendering
	shader := rl.LoadShaderFromMemory("", fsShader.Code)
	defer rl.UnloadShader(shader)

	// The translator name mangles the uniform names, so we
	// need to use the mapped names from the ShaderVariable struct.
	resolutionLoc := rl.GetShaderLocation(shader, fsShader.Variables["iResolution"].MappedName)
	timeLoc := rl.GetShaderLocation(shader, fsShader.Variables["iTime"].MappedName)
	mouseLoc := rl.GetShaderLocation(shader, fsShader.Variables["iMouse"].MappedName)

	if resolutionLoc == -1 || timeLoc == -1 {
		log.Fatalf("Could not find required uniform locations for time or resolution.")
	}

	rl.SetTargetFPS(60)

	// Main Render Loop ---
	for !rl.WindowShouldClose() {
		// Update
		w, h := float32(rl.GetScreenWidth()), float32(rl.GetScreenHeight())
		rl.SetShaderValue(shader, resolutionLoc, []float32{w, h, 0}, rl.ShaderUniformVec3)
		rl.SetShaderValue(shader, timeLoc, []float32{float32(rl.GetTime())}, rl.ShaderUniformFloat)

		// Set mouse position in normalized coordinates
		// mouse pixel coordinates. xy: current (if MLB down), zw: click
		mouseX, mouseY := float32(rl.GetMouseX()), float32(rl.GetMouseY())
		mouseClickX, mouseClickY := float32(rl.GetMouseX()), float32(rl.GetMouseY())
		if mouseLoc != -1 {
			rl.SetShaderValue(shader, mouseLoc, []float32{mouseX, mouseY, mouseClickX, mouseClickY}, rl.ShaderUniformVec4)
		}

		// Draw
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)

		rl.BeginShaderMode(shader)
		// Draw a simple rectangle that covers the entire screen.
		// Raylib will supply the vertexPosition and vertexTexCoord attributes.
		rl.DrawRectangle(0, 0, int32(w), int32(h), rl.White)
		rl.EndShaderMode()

		rl.DrawFPS(10, 10)
		rl.EndDrawing()
	}
}

func main() {
	// The Go runtime needs to be locked to the main thread for graphics libraries like Raylib/OpenGL.
	runtime.LockOSThread()
	runShadertoyExample()
}
```

## API Overview
`goshadertranslator.NewShaderTranslator(ctx context.Context)`

Initializes the wazero runtime and the ANGLE WASM module. Returns a `*ShaderTranslator` instance.

`(st *ShaderTranslator) Close()`

Gracefully shuts down the translator and releases all `wazero` resources. It's important to call this to prevent memory leaks.

`(st *ShaderTranslator) TranslateShader(shaderCode, shaderType, spec, output)`

The core function. It takes the shader source code and strings specifying the shader type ("vertex" or "fragment"), input spec, and output format. It returns a `*Shader` struct or an error.

`goshadertranslator.Shader`

A struct containing the result of a translation.
* `Code string`: The translated, ready-to-use shader code.
* `Variables map[string]ShaderVariable`: A map of active variables in the shader, keyed by their original names.

`goshadertranslator.ShaderVariable`

A struct holding information about a single shader variable.
* `Name string`: The original name of the variable (e.g., `"iResolution"`).
* `MappedName string`: The translated name of the variable (e.g., `"_uiResolution"`). Use this name to get uniform locations.
* `Type uint`: The variable's data type (e.g., `GL_FLOAT_VEC3`).
* ... and other metadata like `Precision`, `StaticUse`, etc.

## Acknowledgements
* This project would not be possible without the incredible work of the Google [ANGLE](https://github.com/google/angle) team.
* The high-performance, dependency-free WASM runtime is provided by [wazero](https://wazero.io/).

## License
This project is licensed under the MIT License.