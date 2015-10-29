package swarmclient

import (
	_ "fmt"
	"github.com/samalba/dockerclient"
)

type ContainerInfo struct {
	dockerclient.ContainerInfo
	Node Node
}

type Node struct {
	ID     string
	IP     string
	Addr   string
	Name   string
	Cpus   int
	Memory float64
	Labels map[string]string
}
