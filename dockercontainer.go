package shipyard

import (
	"fmt"
	"github.com/samalba/dockerclient"
)

type DockerContainer struct {
	ID    string
	Names []string
}

type EngineInfo struct {
	ID   string
	Host string
}

func (d *DockerContainer) String() string {
	return fmt.Sprintf("container id is %s name is %s", d.ID, d.Names)
}

type ContainerConfig struct {
	dockerclient.ContainerConfig
}

type ContainerInfo struct {
	dockerclient.ContainerInfo
	Engine *EngineInfo
}
