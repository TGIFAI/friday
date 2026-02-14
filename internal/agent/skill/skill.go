package skill

type Skill struct {
	isBuiltIn   bool
	Name        string
	Description string
	Metadata    map[string]interface{}
	Content     string
	Path        string
}
