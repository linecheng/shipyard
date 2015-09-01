package shipyard

import (
	"fmt"
	"github.com/samalba/dockerclient"
)

type DockerContainer struct {
	ID    string
	Names []string
}

func (d *DockerContainer) String() string {
	return fmt.Sprintf("container id is %s name is %s", d.ID, d.Names)
}

type ContainerConfig struct {
	dockerclient.ContainerConfig
}
