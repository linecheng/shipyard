package shipyard

import "fmt"

type DockerContainer struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func (d *DockerContainer) String() string {
	return fmt.Sprintf("container id is %s name is %s", d.ID, d.Name)
}
