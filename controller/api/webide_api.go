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

所有对Container的操作都会进行一层Resource的封装



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

//containers/create
func (a *Api) createResource(w http.ResponseWriter, req *http.Request) {
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

	var resource = &resourcing.ContainerResource{
		ResourceID: utils.NewGuid(), Status: resourcing.Avaiable, ContainerID: containerid, CreateTime: time.Now().Local(),
		LastUpdateTime: time.Now().Local(), Image: "", CreatingConfig: config,
	}
	if err := a.manager.SaveResource(resource); err != nil {
		var msg = fmt.Sprintf("资源id= %s, Status =%s , ContainerID=%s  数据库写入失败", resource.ResourceID, resource.Status, resource.ContainerID)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	w.Header().Set("content-type", "application/json")

	w.WriteHeader(http.StatusCreated)
	var data = map[string]interface{}{"Id": resource.ResourceID, "Warnings": []interface{}{}}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

//containers/xxx/inspect
//获取容器的详细信息
//若资源可用，则直接返回信息
//若已镜像化，则返回449 ，同时 返回status和description字段
//若移动中，则返回423 ，资源锁定，同时 返回status和description字段
func (a *Api) inspectResource(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)

	log.Infoln("inspect container ,resource id is " + data["name"])

	var resource, err = a.manager.GetResource(data["name"])

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource == nil {
		http.Error(w, "No such resource "+data["name"], 404) //资源不存在
		return
	}

	swarm, err := a._getSwarmClient()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource.Status == resourcing.Avaiable {

		containerInfo, err := swarm.InspectContainer(resource.ContainerID)
		if err != nil {
			http.Error(w, "resource存在，获得对应Container信息时出现错误。"+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(containerInfo); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		return
	} else {
		log.Infof("资源%s状态为%s", resource.ResourceID, resource.Status)
		var data = map[string]string{"status": resource.Status}
		w.Header().Set("Content-Type", "application/json")
		if resource.Status == resourcing.Image {
			data["description"] = "资源对应容器已经被镜像化，如需查看运行态的信息，请调用start之后，重试"
			w.WriteHeader(449) //Retry With 请求应当在执行完适当的操作后进行重试
		}
		if resource.Status == resourcing.Moving {
			data["description"] = "资源对应容器正在移动中，已被锁定，请稍后再试"
			w.WriteHeader(423) //Locked 当前资源已被锁定
		}

		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	var msg = fmt.Sprintf("%s对应的Resource Status字段值%s有误", resource.ResourceID, resource.Status)
	log.Infoln(msg)
	http.Error(w, msg, http.StatusInternalServerError)

	return
}

// 启动容器
//如果容器当前可用，则启动后返回，
//  如果不可用，若状态是 image则创建新的之后，启动。若移动中，则返回423
// 相对原生api ，多了1个423 资源锁定的状态。同时会返回status字段值为moving.
func (a *Api) startResource(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceID = data["name"]

	log.Infoln("start container ,resource id is " + data["name"])

	var resource, err = a.manager.GetResource(data["name"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource == nil {
		http.Error(w, "No such resource "+resource.ResourceID, 404) //资源不存在
		return
	}

	swarm, err := a._getSwarmClient()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource.Status == resourcing.Avaiable {
		log.Infof("资源%s可用，将尝试启动", resource.ResourceID)
		//转发到原生api上进行启动
		a._redirectToStartContainer(resource.ContainerID, w, req)
		return
	}

	if resource.Status == resourcing.Image {
		log.Infof("资源%s状态为%s", resource.ResourceID, resource.Status)
		//根据imagename创建容器
		containerid, err := a._createContainerByImage(swarm, resource.Image, resource.CreatingConfig)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		dbResource, err := a.manager.GetResource(resource.ResourceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dbResource.Status = resourcing.Avaiable
		dbResource.ContainerID = containerid
		dbResource.LastUpdateTime = time.Now().Local()
		if err = a.manager.UpdateResource(dbResource.ResourceID, dbResource); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		//容器创建成功之后，转发到原生的启动接口上
		log.Infoln("开始启动容器->" + containerid)
		a._redirectToStartContainer(containerid, w, req)
		return
	}

	if resource.Status == resourcing.Moving {

		log.Infof("资源%s状态为%s, 持续查询中", resource.ResourceID, resource.Status)
		c_done := make(chan map[string]string)
		go a.manager.WaitUntilResourceAvaiable(resourceID, (time.Duration(a.waitMovingTimeout) * time.Second), c_done)
		done := <-c_done
		if done["done"] != "true" {
			log.Infof("资源%s等待movint->avaiable超时", resource.ResourceID)
			http.Error(w, "资源处于Moving中，等待超 时 :"+done["error"], http.StatusInternalServerError)
			return
		} else {
			log.Infof("资源%s状态已为avaiable, ContainerID = ", done["containerID"])
			a._redirectToStartContainer(done["containerID"], w, req)
			return
		}

	}

	var msg = fmt.Sprintf("%s对应的Resource Status字段值%s有误", resource.ResourceID, resource.Status)
	log.Infoln(msg)
	http.Error(w, msg, http.StatusInternalServerError)

	return
}

func (a *Api) _redirectToStartContainer(containerID string, w http.ResponseWriter, req *http.Request) {
	var err error
	req.URL, err = url.ParseRequestURI(a.dUrl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var segments []string = strings.Split(req.RequestURI, "/")
	if len(segments) > 3 {
		segments[2] = containerID
	}

	req.RequestURI = strings.Join(segments, "/")
	log.Debugln("转发至" + req.URL.String() + req.RequestURI + "启动container")
	a.fwd.ServeHTTP(w, req)
}

func (a *Api) redirectToContainer(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceid = data["name"]
	log.Debugln("开始转发对" + req.RequestURI + "请求")
	var resource, err = a.manager.GetResource(resourceid)
	if err != nil {
		http.Error(w, "资源id="+resourceid+"对应记录获取错误"+err.Error(), http.StatusNotFound)
		return
	}
	if resource == nil {
		http.Error(w, "资源id="+resourceid+"对应记录不存在", http.StatusNotFound)
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
	if len(segments) > 3 {
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

//func (a *Api) _recursiveToStartContainer(docker *swarmclient.SwarmClient, containerid string) (*swarmclient.ContainerInfo, error) {

//	var baseerror = errors.New("StartContainer 出错,ContainerID=" + containerid)

//	var containerInfo, err = docker.InspectContainer(containerid)
//	if err != nil {
//		return nil, utils.Errors(baseerror, err)
//	}
//	if containerInfo.State.Running {
//		return containerInfo, nil
//	}

//	if count >= MAXCOUNT {
//		return nil, errors.New("容器尝试启动超过最大次数，启动失败")
//	}

//	if err := docker.StartContainer(containerid, nil); err != nil {
//		return nil, utils.Errors(baseerror, err)
//	}
//	log.Infof(" 第 %d次启动%s 容器", count+1, containerid)
//	count++

//	containerInfo, err = docker.InspectContainer(containerid)
//	if err != nil {
//		return nil, utils.Errors(baseerror, err)
//	}
//	if containerInfo.State.Running {
//		log.Infof("容器%s启动成功", containerid)
//		count = 0
//		return containerInfo, nil
//	} else {
//		return a._recursiveToStartContainer(docker, containerid)
//	}
//}

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

func (a *Api) _createContainerByImage(swarm *swarmclient.SwarmClient, imageName string, creatingConfig *dockerclient.ContainerConfig) (string, error) {
	//	var image, err = a.appendLocalRegistryToImageName(imageName) // 172.16.150.12:5000/nndtdx/workspaceName:commitid
	//	if err != nil {
	//		return "", nil
	//	}
	//	var image = imageName
	//	var config = &dockerclient.ContainerConfig{
	//		Image: image,
	//	}
	log.Debugln("creatingconfig-->")
	log.Debugln(creatingConfig)

	creatingConfig.Image = imageName

	log.Infoln("开始从镜像" + imageName + "创建Container")
	log.Debugln("create container by image , config->  :", creatingConfig)
	containerid, err := swarm.CreateContainer(creatingConfig, "") //先在本地找，然后会根据指定的 ImageFullName 来在相应的Registry中Pul
	if err != nil {
		return "", err
	}
	log.Infof("容器创建成功，得到ContainerID=%s", containerid)

	return containerid, nil

}

func (a *Api) abandonContainer(w http.ResponseWriter, req *http.Request) {

	var data = mux.Vars(req)
	var resourceid = data["name"]
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
