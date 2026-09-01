package agents

type llmLink struct {
	ExistingArticleID int64  `json:"existing_article_id"`
	ExactPhraseInText string `json:"exact_phrase_in_text"`
}

type OKFMetadata struct {
	Type        string           `json:"type" yaml:"type"`
	Title       string           `json:"title" yaml:"title"`
	Description string           `json:"description" yaml:"description"`
	Source      string           `json:"source,omitempty" yaml:"source,omitempty"`
	Tags        []string         `json:"tags" yaml:"tags"`
	Generated   OKFGeneratedInfo `json:"-" yaml:"generated"`
}

type OKFGeneratedInfo struct {
	By string `yaml:"by"`
	At string `yaml:"at"`
}

type OKFFrontmatterResponse struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type jsonSchemaField struct {
	Type                 string                     `json:"type"`
	Properties           map[string]jsonSchemaField `json:"properties,omitempty"`
	Items                *jsonSchemaField           `json:"items,omitempty"`
	Required             []string                   `json:"required,omitempty"`
	AdditionalProperties bool                       `json:"additionalProperties"`
}

type jsonSchemaDefinition struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema jsonSchemaField `json:"schema"`
}

type responseFormat struct {
	Type       string                `json:"type"`
	JSONSchema *jsonSchemaDefinition `json:"json_schema,omitempty"`
}

type UnifiedPipelineResponse struct {
	Summary       string                  `json:"summary,omitempty"`
	Frontmatter   *OKFFrontmatterResponse `json:"frontmatter,omitempty"`
	LinksToInject []llmLink               `json:"links_to_inject,omitempty"`
}

type pipelineOpenRouterRequest struct {
	Model          string          `json:"model"`
	Messages       []interface{}   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}
