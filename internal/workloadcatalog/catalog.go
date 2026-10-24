// Package workloadcatalog describes the linked workload API clients without
// importing those clients into the management service.
package workloadcatalog

type Flag struct {
	Hidden      bool   `json:"hidden,omitempty"`
	Name        string `json:"name"`
	Short       string `json:"short,omitempty"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Boolean     bool   `json:"boolean,omitempty"`
}
type Command struct {
	MinArgs      int       `json:"min_args,omitempty"`
	MaxArgs      *int      `json:"max_args,omitempty"`
	Interspersed bool      `json:"interspersed,omitempty"`
	Hidden       bool      `json:"hidden,omitempty"`
	Name         string    `json:"name"`
	Aliases      []string  `json:"aliases,omitempty"`
	Description  string    `json:"description"`
	Usage        string    `json:"usage"`
	Runnable     bool      `json:"runnable"`
	Flags        []Flag    `json:"flags,omitempty"`
	Children     []Command `json:"children,omitempty"`
}
