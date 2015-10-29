package containerresourcing

import (
	_ "fmt"
	"time"
)

// ContainerResource status
const (
	Avaiable string = "avaiable"
	Image    string = "image"
	Moving   string = "moving"
)

type ContainerResource struct {
	ID             string
	ContainerID    string
	Status         string
	Image          string
	LastUpdateTime time.Time
	CreateTime     time.Time
}
