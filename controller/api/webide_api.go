package api

import (
	_ "bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"errors"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/samalba/dockerclient"
	resourcing "github.com/shipyard/shipyard/containerresourcing"
	"github.com/shipyard/shipyard/swarmclient"
	"github.com/shipyard/shipyard/utils"
	_ "io"
	"io/ioutil"
	"net/http"
	"time"
)

/*

webide对后端 集群 的操作

    申请，连接，放弃，状态
     ws/xx/container/apply
     ws/xx/container/connect
     ws/xx/container/abandon
     ws/xx/container/status

资源申请的时候，则有资源参数（Container Config）

后端 Shipyard表 BackendResoures表
ResourceID , ContainerID, Status , Image

Apply后 返回的ContainerID， 生成ResourceID, Status= Active , Image=nil。 将ResouceID返回给 WebIDE.
Connect向后端提供ResourceID, 后端对资源进行处理（如果停止则进行启动等），处理成功后，将 资源的详细信息返回。
Abandon 向后端提供ResourceID, 后端将会对 Container执行停止等操作。
Status 向后端提供ResourceID，后端取得该ResourceID对应的ContainerID，返回状态。

对于新来的用户 ，进行apply即可，对于旧有用户，则进行connect操作， 后端自主来根据自己记录的Resource状态，进行相应的操作，或直接启动，或 先进行移动然后再启动，或先进行调度，然后pull,然后run .

*/

//create 之后 start并且返回详细的Container信息，同时插入数据库 相应的数据
//?name=xxx

func (a *Api) _getSwarmClient() (*swarmclient.SwarmClient, error) {
	_docker, err := dockerclient.NewDockerClient(a.dUrl, nil)
	if err != nil {
		return nil, errors.New("getSwarmClient出现错误" + err.Error())
	}
	swarm := swarmclient.NewSwarmClientByDockerClient(_docker)
	return swarm, nil
}

func (a *Api) applyContainer(w http.ResponseWriter, req *http.Request) {
	log.Infoln("begin to Apply Container")
	swarm, err := a._getSwarmClient()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	config := &dockerclient.ContainerConfig{}
	names := req.URL.Query()["name"]
	name := ""
	if len(names) > 0 {
		name = names[0]
	}

	bts, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = json.Unmarshal(bts, config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Infoln("begin to create Container")
	containerid, err := swarm.CreateContainer(config, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	containerinfo, err := a._recursiveToStartContainer(swarm, containerid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var resource = &resourcing.ContainerResource{
		ID: utils.NewGuid(), Status: resourcing.Avaiable, ContainerID: containerid, CreateTime: time.Now(), LastUpdateTime: time.Now(), Image: "",
	}
	if err := a.manager.SaveResource(resource); err != nil {
		var msg = fmt.Sprintf("资源id= %s, Status =%s , ContainerID=%s  数据库写入失败", resource.ID, resource.Status, resource.ContainerID)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.Header().Set("resourceid", resource.ID)
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(containerinfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *Api) connectContainer(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)

	log.Infoln("connect container ,resource id is " + data["id"])

	var resource, err = a.manager.GetResource(data["id"])
	log.Infoln(resource)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	swarm, err := a._getSwarmClient()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource.Status == resourcing.Avaiable {
		log.Infof("资源%s可用，将尝试启动", resource.ID)
		containerinfo, err := a._recursiveToStartContainer(swarm, resource.ContainerID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(containerinfo); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		return
	}

	if resource.Status == resourcing.Image {
		log.Infof("资源%s状态为%s", resource.ID, resource.Status)
		containerid, err := a._createContainerByImage(swarm, resource.Image)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		dbResource, err := a.manager.GetResource(resource.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dbResource.Status = resourcing.Avaiable
		dbResource.ContainerID = containerid
		dbResource.LastUpdateTime = time.Now()
		if err = a.manager.UpdateResource(dbResource.ID, dbResource); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Infoln("开始启动容器->" + containerid)
		containerinfo, err := a._recursiveToStartContainer(swarm, containerid)
		if err := json.NewEncoder(w).Encode(containerinfo); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	if resource.Status == resourcing.Moving {
		// 等待还是返回 错误信息呢？
		http.Error(w, "正在移动中，请等待", http.StatusInternalServerError)
		return
	}

	var msg = fmt.Sprintf("%s对应的Resource Status字段值%s有误", resource.ID, resource.Status)
	log.Infoln(msg)
	http.Error(w, msg, http.StatusInternalServerError)

	return
}

func (a *Api) redirectToContainer(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceid = data["id"]
	log.Infoln("开始转发对" + req.RequestURI + "请求")
	var resource, err = a.manager.GetResource(resourceid)
	if err != nil {
		http.Error(w, "资源id="+resourceid+"对应记录获取错误"+err.Error(), http.StatusNotFound)
		return
	}
	if resource == nil {
		http.Error(w, "资源id="+resourceid+"对应记录不存在"+err.Error(), http.StatusNotFound)
		return
	}

	if resource.Status != resourcing.Avaiable {
		http.Error(w, "资源id="+resourceid+"对应的资源不可用，状态="+resource.Status+"，请确保客户端在Connect状态下进行操作", http.StatusForbidden)
		return
	}

	req.URL, err = url.ParseRequestURI(a.dUrl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var segments []string = strings.Split(req.RequestURI, "/")
	if len(segments) > 3 && segments[1] == "resources" {
		segments[1] = "containers"
		segments[2] = resource.ContainerID
	}

	req.RequestURI = strings.Join(segments, "/")
	log.Infoln("转发至" + req.URL.String() + req.RequestURI)
	a.fwd.ServeHTTP(w, req)
}

const MAXCOUNT int = 3

var count int = 0

func (a *Api) _recursiveToStartContainer(docker *swarmclient.SwarmClient, containerid string) (*swarmclient.ContainerInfo, error) {

	var baseerror = errors.New("StartContainer 出错,ContainerID=" + containerid)

	var containerInfo, err = docker.InspectContainer(containerid)
	if err != nil {
		return nil, utils.Errors(baseerror, err)
	}
	if containerInfo.State.Running {
		return containerInfo, nil
	}

	if count >= MAXCOUNT {
		return nil, errors.New("容器尝试启动超过最大次数，启动失败")
	}

	if err := docker.StartContainer(containerid, nil); err != nil {
		return nil, utils.Errors(baseerror, err)
	}
	log.Infof(" 第 %d次启动%s 容器", count+1, containerid)
	count++

	containerInfo, err = docker.InspectContainer(containerid)
	if err != nil {
		return nil, utils.Errors(baseerror, err)
	}
	if containerInfo.State.Running {
		log.Infof("容器%s启动成功", containerid)
		count = 0
		return containerInfo, nil
	} else {
		return a._recursiveToStartContainer(docker, containerid)
	}
}

func (a *Api) appendLocalRegistryToImageName(imageName string) (string, error) {
	if imageName == "" {
		return "", errors.New("镜像名称不能为空")
	}
	var prefix = a.registryAddr
	if strings.HasPrefix(imageName, "/") {
		return prefix + imageName, nil
	}
	return prefix + "/" + imageName, nil
}

func (a *Api) _createContainerByImage(swarm *swarmclient.SwarmClient, imageName string) (string, error) {
	var image, err = a.appendLocalRegistryToImageName(imageName) // 172.16.150.12:5000/nndtdx/workspaceName:commitid
	if err != nil {
		return "", nil
	}
	var config = &dockerclient.ContainerConfig{
		Image: image,
	}
	log.Infoln("开始从镜像" + image + "创建Container")
	containerid, err := swarm.CreateContainer(config, "") //先在本地找，然后会根据指定的 ImageFullName 来在相应的Registry中Pul
	if err != nil {
		return "", err
	}
	log.Infof("容器创建成功，得到ContainerID=%s", containerid)

	return containerid, nil

}

func (a *Api) abandonContainer(w http.ResponseWriter, req *http.Request) {

	var data = mux.Vars(req)
	var resourceid = data["id"]
	log.Infoln("放弃对资源" + resourceid + "的持有")
	var resource, err = a.manager.GetResource(resourceid)
	if err != nil {
		http.Error(w, "资源id="+resourceid+"对应记录获取错误"+err.Error(), http.StatusNotFound)
		return
	}
	if resource == nil {
		http.Error(w, "资源id="+resourceid+"对应记录不存在"+err.Error(), http.StatusNotFound)
		return
	}

	swam, err := a._getSwarmClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = swam.StopContainer(resource.ContainerID, 0)
	if err != nil {
		http.Error(w, "StopContainer出现错误"+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusNoContent)
	return
}
