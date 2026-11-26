package config

// Tool is a detected runtime and the executable that `which` would resolve.
type Tool struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// knownTools lists the runtimes cln detects, in the order they are reported.
func knownTools() []string {
	return []string{"node", "bun"}
}

// DetectTools returns the known tools that lookPath can resolve, together
// with the resolved executable. Pass exec.LookPath to search PATH the same
// way `which` does.
func DetectTools(lookPath func(file string) (string, error)) []Tool {
	found := []Tool{}
	for _, name := range knownTools() {
		if path, err := lookPath(name); err == nil {
			found = append(found, Tool{Name: name, Path: path})
		}
	}
	return found
}
