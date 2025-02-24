package types

type FlowChild struct {
	Function string `json:"function"`
}

type Args struct {
	Main      []string                     `json:"main"`
	Functions map[string]map[string]string `json:"functions"`
}

type Flow struct {
	Args     Args                 `json:"args"`
	Children map[string]FlowChild `json:"children,omitempty"`
}

type Flows struct {
	Flows map[string]Flow `json:"flows"`
}

type FlowOutput struct {
	Data     map[string]interface{} `json:"data"`
	Function string                 `json:"function"`
}

type FlowInput struct {
	Args     map[string]interface{} `json:"args"`
	Children map[string]*FlowOutput `json:"children"`
}
