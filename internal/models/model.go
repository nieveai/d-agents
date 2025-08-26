package models

type Model struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	ModelID  string `json:"model_id"`
	APIURL   string `json:"api_url,omitempty"`
	APISpec  string `json:"api_spec,omitempty"`
}
