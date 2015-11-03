package containerresourcing

import (
	_ "fmt"
	"github.com/samalba/dockerclient"
	"time"
)

// ContainerResource status
const (
	Avaiable string = "avaiable"
	Image    string = "image"
	Moving   string = "moving"
)

type ContainerResource struct {
	ResourceID     string
	ContainerID    string
	Status         string
	Image          string
	LastUpdateTime time.Time
	CreateTime     time.Time
	CreatingConfig *dockerclient.ContainerConfig
}
