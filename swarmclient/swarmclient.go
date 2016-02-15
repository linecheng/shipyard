package swarmclient

import (
	"bytes"
	//"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/samalba/dockerclient"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type SwarmClient struct {
	dockerclient.DockerClient
}

// func NewSwarmClient(swarmUrl string, tlsConfig *tls.Config) (*SwarmClient, error) {
// 	var docker, err = dockerclient.NewDockerClient(swarmUrl, tlsConfig)

// 	if err != nil {
// 		return nil, err
// 	}

// 	return &SwarmClient{
// 		DockerClient: *docker,
// 	}, nil
// }

func NewSwarmClientByDockerClient(docker *dockerclient.DockerClient) *SwarmClient {
	return &SwarmClient{DockerClient: *docker}
}

func (client *SwarmClient) InspectContainer(id string) (*ContainerInfo, error) {
	uri := fmt.Sprintf("/%s/containers/%s/json", dockerclient.APIVersion, id)
	data, err := client.doRequest("GET", uri, nil, nil)
	if err != nil {
		return nil, err
	}
	info := &ContainerInfo{}
	err = json.Unmarshal(data, info)
	if err != nil {
		return nil, err
	}
	return info, nil
}
func (client *SwarmClient) doRequest(method string, path string, body []byte, headers map[string]string) ([]byte, error) {
	b := bytes.NewBuffer(body)

	reader, err := client.doStreamRequest(method, path, b, headers)
	if err != nil {
		return nil, err
	}

	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (client *SwarmClient) doStreamRequest(method string, path string, in io.Reader, headers map[string]string) (io.ReadCloser, error) {
	if (method == "POST" || method == "PUT") && in == nil {
		in = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, client.URL.String()+path, in)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	if headers != nil {
		for header, value := range headers {
			req.Header.Add(header, value)
		}
	}
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		if !strings.Contains(err.Error(), "connection refused") && client.TLSConfig == nil {
			return nil, fmt.Errorf("%v. Are you trying to connect to a TLS-enabled daemon without TLS?", err)
		}
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, dockerclient.ErrNotFound
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, Error{StatusCode: resp.StatusCode, Status: resp.Status, msg: string(data)}
	}

	return resp.Body, nil
}

type Error struct {
	StatusCode int
	Status     string
	msg        string
}

func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Status, e.msg)
}
