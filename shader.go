package goshadertranslator

type ShaderVariable struct {
	Active     bool   `json:"active"`
	IsRowMajor bool   `json:"is_row_major"`
	MappedName string `json:"mapped_name"`
	Name       string `json:"name"`
	Precision  uint   `json:"precision_enum"`
	StaticUse  bool   `json:"static_use"`
	Type       uint   `json:"type_enum"`
	Category   string `json:"category"`
}

type Shader struct {
	Code      string                    `json:"code"`
	Variables map[string]ShaderVariable `json:"variables,omitempty"`
}

func newShader(response map[string]interface{}) *Shader {
	fsResultPayload, _ := response["result"].(map[string]interface{})
	active_variables, _ := fsResultPayload["active_variables"].(map[string]interface{})

	// iterate over the active variables and convert them to ShaderVariable
	variables := make(map[string]ShaderVariable)
	for name, varData := range active_variables {
		// name is the category name, varData is the slice of variable data
		for _, data := range varData.([]interface{}) {
			variableMap, ok := data.(map[string]interface{})
			if !ok {
				continue // skip if the data is not a map
			}
			variable := ShaderVariable{
				Active:     variableMap["active"].(bool),
				IsRowMajor: variableMap["is_row_major"].(bool),
				MappedName: variableMap["mapped_name"].(string),
				Name:       variableMap["name"].(string),
				Precision:  uint(variableMap["precision_enum"].(float64)),
				StaticUse:  variableMap["static_use"].(bool),
				Type:       uint(variableMap["type_enum"].(float64)),
				Category:   name,
			}
			variables[variable.Name] = variable
		}
	}

	return &Shader{
		Code:      fsResultPayload["object_code"].(string),
		Variables: variables,
	}
}
