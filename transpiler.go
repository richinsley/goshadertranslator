package goshadertranslator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	_ "embed"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type ShaderSpec string

const (
	ShaderSpecGLES2  ShaderSpec = "gles2"
	ShaderSpecGLES3  ShaderSpec = "gles3"
	ShaderSpecGLES31 ShaderSpec = "gles31"
	ShaderSpecGLES32 ShaderSpec = "gles32"
	ShaderSpecWebGL  ShaderSpec = "webgl"
	ShaderSpecWebGL2 ShaderSpec = "webgl2"
	ShaderSpecWebGL3 ShaderSpec = "webgl3"
	ShaderSpecWebGLN ShaderSpec = "webgln" // WebGL 1.0 no highp
)

type OutputFormat string

const (
	OutputFormatESSL    OutputFormat = "essl"
	OutputFormatGLSL    OutputFormat = "glsl"
	OutputFormatGLSL130 OutputFormat = "glsl130"
	OutputFormatGLSL140 OutputFormat = "glsl140"
	OutputFormatGLSL150 OutputFormat = "glsl150"
	OutputFormatGLSL330 OutputFormat = "glsl330"
	OutputFormatGLSL400 OutputFormat = "glsl400"
	OutputFormatGLSL410 OutputFormat = "glsl410"
	OutputFormatGLSL420 OutputFormat = "glsl420"
	OutputFormatGLSL430 OutputFormat = "glsl430"
	OutputFormatGLSL440 OutputFormat = "glsl440"
	OutputFormatGLSL450 OutputFormat = "glsl450"
)

//go:embed wasm_out/angle_shader_translator_standalone.wasm
var wasmByteCode []byte

// ShaderTranslator wraps the wazero runtime and ANGLE WASM module.
type ShaderTranslator struct {
	runtime     wazero.Runtime
	module      api.Module
	ctx         context.Context
	closed      bool
	initializer api.Function
	finalizer   api.Function
	invoker     api.Function
	malloc      api.Function
	free        api.Function
}

type TranslateRequestParams struct {
	ShaderCodeBase64     string          `json:"shader_code_base64"`
	ShaderType           string          `json:"shader_type"`
	Spec                 ShaderSpec      `json:"spec"`
	Output               OutputFormat    `json:"output"`
	PrintActiveVariables bool            `json:"print_active_variables"`
	CompileOptions       map[string]bool `json:"compile_options"`
}

type JSONRPCRequest struct {
	JsonRPC string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Method  string                 `json:"method"`
	Params  TranslateRequestParams `json:"params"`
}

// NewShaderTranslator initializes the wazero runtime, loads the WASM module,
// and prepares it for use.
func NewShaderTranslator(ctx context.Context) (*ShaderTranslator, error) {
	r := wazero.NewRuntime(ctx)

	// we'll need to instantiate WASI because the WASM module was
	// compiled with dependencies on it (e.g., for libc functions).
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	compiledModule, err := r.CompileModule(ctx, wasmByteCode)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("failed to compile wasm module: %w", err)
	}

	moduleConfig := wazero.NewModuleConfig().WithStartFunctions()

	module, err := r.InstantiateModule(ctx, compiledModule, moduleConfig)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("failed to instantiate wasm module: %w", err)
	}

	initializer := module.ExportedFunction("initialize")
	finalizer := module.ExportedFunction("finalize")
	invoker := module.ExportedFunction("invoke")
	malloc := module.ExportedFunction("malloc")
	free := module.ExportedFunction("free")

	if invoker == nil || malloc == nil || free == nil || initializer == nil || finalizer == nil {
		r.Close(ctx)
		return nil, fmt.Errorf("one or more required library functions not exported from wasm module")
	}

	result, err := initializer.Call(ctx)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("failed to call 'initialize' function: %w", err)
	}
	if result[0] == 0 {
		r.Close(ctx)
		return nil, fmt.Errorf("the ANGLE library's 'initialize' function failed")
	}

	return &ShaderTranslator{
		runtime:     r,
		module:      module,
		ctx:         ctx,
		closed:      false,
		initializer: initializer,
		finalizer:   finalizer,
		invoker:     invoker,
		malloc:      malloc,
		free:        free,
	}, nil
}

// Close gracefully finalizes the ANGLE library and releases wazero resources.
func (st *ShaderTranslator) Close() error {
	if st.closed {
		return nil
	}
	if _, err := st.finalizer.Call(st.ctx); err != nil {
		log.Printf("warning: call to wasm finalizer failed: %v", err)
	}
	if err := st.runtime.Close(st.ctx); err != nil {
		return fmt.Errorf("failed to close wazero runtime: %w", err)
	}
	st.closed = true
	return nil
}

// TranslateShader translates shader code by invoking the WASM module.
func (st *ShaderTranslator) TranslateShader(shaderCode string, shaderType string, spec ShaderSpec, output OutputFormat) (*Shader, error) {
	if st.closed {
		return nil, fmt.Errorf("translator has been closed")
	}

	shaderCodeB64 := base64.StdEncoding.EncodeToString([]byte(shaderCode))
	requestPayload := JSONRPCRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "translate",
		Params: TranslateRequestParams{
			ShaderCodeBase64:     shaderCodeB64,
			ShaderType:           shaderType,
			Spec:                 spec,
			Output:               output,
			PrintActiveVariables: true,
			CompileOptions:       map[string]bool{"objectCode": true},
		},
	}
	requestBytes, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	requestPtr, err := st.writeStringToMemory(requestBytes)
	if err != nil {
		return nil, err
	}
	defer st.free.Call(st.ctx, requestPtr)

	result, err := st.invoker.Call(st.ctx, requestPtr)
	if err != nil {
		return nil, fmt.Errorf("wasm invoke call failed: %w", err)
	}
	responsePtr := result[0]
	if responsePtr == 0 {
		return nil, fmt.Errorf("wasm invoke function returned a null pointer")
	}

	responseBytes, err := st.readStringFromMemory(uint32(responsePtr))
	if err != nil {
		return nil, err
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wasm response: %w", err)
	}

	serr, _ := responseMap["error"].(map[string]interface{})
	if serr != nil {
		errorMessage, _ := serr["message"].(string)
		data, _ := serr["data"].(map[string]interface{})
		log, _ := data["info_log"].(string)
		return nil, fmt.Errorf("%s\n%s", errorMessage, log)
	}
	return newShader(responseMap), nil
}

func (st *ShaderTranslator) writeStringToMemory(data []byte) (uint64, error) {
	byteCount := uint64(len(data))
	results, err := st.malloc.Call(st.ctx, byteCount+1)
	if err != nil {
		return 0, fmt.Errorf("wasm malloc call failed: %w", err)
	}
	ptr := results[0]
	if ptr == 0 {
		return 0, fmt.Errorf("wasm malloc failed to allocate memory")
	}
	if !st.module.Memory().Write(uint32(ptr), data) {
		return 0, fmt.Errorf("failed to write to wasm memory")
	}
	if !st.module.Memory().WriteByte(uint32(ptr+byteCount), 0) {
		return 0, fmt.Errorf("failed to write null terminator to wasm memory")
	}
	return ptr, nil
}

func (st *ShaderTranslator) readStringFromMemory(ptr uint32) ([]byte, error) {
	mem := st.module.Memory()
	memBuffer, ok := mem.Read(ptr, mem.Size()-ptr)
	if !ok {
		return nil, fmt.Errorf("failed to read from wasm memory")
	}
	var nullTerminatorIndex = -1
	for i, b := range memBuffer {
		if b == 0 {
			nullTerminatorIndex = i
			break
		}
	}
	if nullTerminatorIndex == -1 {
		return nil, fmt.Errorf("string from wasm is not null-terminated")
	}
	return memBuffer[:nullTerminatorIndex], nil
}
