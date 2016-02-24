package api

import (
	_ "bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"errors"
	_ "io"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/samalba/dockerclient"
	"github.com/satori/go.uuid"
	resourcing "github.com/shipyard/shipyard/containerresourcing"
	"github.com/shipyard/shipyard/controller/requniqueness"
	"github.com/shipyard/shipyard/swarmclient"
	_ "github.com/shipyard/shipyard/utils"
)

/*
所有对Container的操作都会进行一层Resource的封装
*/

func (a *Api) _getSwarmClient() (*swarmclient.SwarmClient, error) {
	_docker := a.manager.DockerClient()
	swarm := swarmclient.NewSwarmClientByDockerClient(_docker)
	return swarm, nil
}

//containers/create
func (a *Api) createResource(w http.ResponseWriter, req *http.Request) {
	swarm, err := a._getSwarmClient()

	if err != nil {
		log.Error("a._getSwarmClient Error", err.Error())
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
		log.Error("ioutil.ReadAll() Error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = json.Unmarshal(bts, config)
	if err != nil {
		log.Error("json.Unmarshall() Error: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Infoln("begin to create Container")
	containerid, err := swarm.CreateContainer(config, name, nil)
	if err != nil {
		log.Error("swarm.CreateContainer() Error :", "name = ", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var resource = &resourcing.ContainerResource{
		ResourceID: uuid.NewV4().String(), Status: resourcing.Avaiable, ContainerID: containerid, CreateTime: time.Now().Local(),
		LastUpdateTime: time.Now().Local(), Image: "", CreatingConfig: config,
	}
	if err := a.manager.SaveResource(resource); err != nil {
		var msg = fmt.Sprintf("资源id= %s, Status =%s , ContainerID=%s  数据库写入失败", resource.ResourceID, resource.Status, resource.ContainerID)
		log.Error("SaveResource Error", msg, "Error :", err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	log.Info("Resource Create Success , ResourceId=", resource.ResourceID, " container id is ", resource.ContainerID)

	w.Header().Set("content-type", "application/json")

	w.WriteHeader(http.StatusCreated)
	var data = map[string]interface{}{"Id": resource.ResourceID, "Warnings": []interface{}{}}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error("json.NewEncoder() Error:", err.Error())
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

	var resource, err = a.manager.GetResource(data["name"])

	if err != nil {
		log.WithField("data[name]", data["name"]).Error("GetResource Error:", err.Error)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource == nil {
		log.WithField("data[name]", data["name"]).Error("资源不存在")
		http.Error(w, "No such resource "+data["name"], 404) //资源不存在
		return
	}

	swarm, err := a._getSwarmClient()

	if err != nil {
		log.WithField("data[name]", data["name"]).Error("_getSwarmClient Error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource.Status == resourcing.Avaiable {

		containerInfo, err := swarm.InspectContainer(resource.ContainerID)
		if err != nil {
			log.WithField("containerId", resource.ContainerID).Error("swarm.InspectContainer Error:", err.Error())
			http.Error(w, "resource存在，获得对应Container信息时出现错误。ContainerID = "+resource.ContainerID+" Error = "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(containerInfo); err != nil {
			log.Error("Encoder Error :", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		log.Infof("资源%s状态为%s", resource.ResourceID, resource.Status)
		var data = map[string]string{"status": resource.Status}
		w.Header().Set("Content-Type", "application/json")
		if resource.Status == resourcing.Image {
			data["description"] = "资源对应容器已经被镜像化，如需查看运行态的信息，请调用start之后，重试"
			w.WriteHeader(449) //Retry With 请求应当在执行完适当的操作后进行重试
			log.WithField("resourceId", resource.ResourceID).WithField("containerId", resource.ContainerID).Info("Status=img")
		}
		if resource.Status == resourcing.Moving {
			data["description"] = "资源对应容器正在移动中，已被锁定，请稍后再试"
			w.WriteHeader(423) //Locked 当前资源已被锁定
			log.WithField("resourceId", resource.ResourceID).WithField("containerId", resource.ContainerID).Info("Status=moving")
		}

		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Error("Encode Error", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// 启动容器
//如果容器当前可用，则启动后返回，
//  如果不可用，若状态是 image则创建新的之后，启动。若移动中，则返回423
// 相对原生api ，多了1个423 资源锁定的状态。同时会返回status字段值为moving.
func (a *Api) startResource(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceID = data["name"]

	var cxtLog = log.WithField("resourceID", resourceID)

	alreadyError := requniqueness.Handle("/startresource", resourceID) //持有当前资源，禁止其他请求并发再次持有
	if alreadyError != nil {
		cxtLog.Infoln(resourceID, "StartResource操作正在处理中")
		w.Write([]byte("正在处理中，请稍候"))
		return
	}

	defer requniqueness.Release("/startresource", resourceID)

	var resource, err = a.manager.GetResource(data["name"])
	if err != nil {
		cxtLog.Error("GetResource Error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource == nil {
		cxtLog.Error("No Such Resource")
		http.Error(w, "No such resource "+resourceID, 404) //资源不存在
		return
	}

	cxtLog = cxtLog.WithField("containerID", resource.ContainerID)

	swarm, err := a._getSwarmClient()

	if err != nil {
		cxtLog.Error("_getSwarmClient Error :", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resource.Status == resourcing.Avaiable {
		cxtLog.Info("Status is Avaiable")
		a._redirectToContainer(resource.ContainerID, w, req)
		return
	}

	if resource.Status == resourcing.Image {
		cxtLog.Info("Status=image , now Create by image ", resource.Image)
		containerid, err := a._createContainerByImage(swarm, resource.Image, resource.CreatingConfig)
		if err != nil {
			cxtLog.WithField("resource.Image", resource.Image).Error("_createContainerByImage Error:")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		dbResource, err := a.manager.GetResource(resource.ResourceID)
		if err != nil {
			cxtLog.Error("GetResource Error:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cxtLog = cxtLog.WithField("containerID", containerid)
		dbResource.Status = resourcing.Avaiable
		dbResource.ContainerID = containerid
		dbResource.LastUpdateTime = time.Now().Local()
		if err = a.manager.UpdateResource(dbResource.ResourceID, dbResource); err != nil {
			cxtLog.Error("UpdateResource Error", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cxtLog.Info("redirect to start container")

		//容器创建成功之后，转发到原生的启动接口上
		a._redirectToContainer(containerid, w, req)
		return
	}

	if resource.Status == resourcing.Moving {

		cxtLog.Infof("Status=moving,handle for wait moving.")
		c_done := make(chan map[string]string)
		go a.manager.WaitUntilResourceAvaiable(resourceID, (time.Duration(a.waitMovingTimeout) * time.Second), c_done)
		done := <-c_done
		if done["done"] != "true" {
			cxtLog.Warn("handle for wait moving, time out")
			http.Error(w, "资源处于Moving中，等待超 时 :"+done["error"], http.StatusInternalServerError)
			return
		} else {
			cxtLog.Infof("status changed : moving->avaiable")
			a._redirectToContainer(done["containerID"], w, req)
			return
		}

	}

	var msg = fmt.Sprintf("%s对应的Resource Status字段值%s有误", resource.ResourceID, resource.Status)
	cxtLog.WithField("status", resource.Status).Error("status value error")
	http.Error(w, msg, http.StatusInternalServerError)

	return
}

func (a *Api) _redirectToContainer(containerID string, w http.ResponseWriter, req *http.Request) {
	var cxtLog = log.WithField("containerId", containerID)

	var err error
	req.URL, err = url.ParseRequestURI(a.dUrl)
	if err != nil {
		cxtLog.WithField("a.dUrl", a.dUrl).Error("parseRequestURI Error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var segments []string = strings.Split(req.RequestURI, "/")
	if len(segments) >= 3 {
		segments[2] = containerID
	}

	req.RequestURI = strings.Join(segments, "/")

	cxtLog.Info("REDIRECT  ", req.Method, ":", req.RequestURI)
	a.fwd.ServeHTTP(w, req)
}

//将会将/XX/{name} 替换成 /containers/containerID
func (a *Api) redirectToContainer(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceid = data["name"]

	var cxtLog = log.WithField("resourceId", resourceid)

	var resource, err = a.manager.GetResource(resourceid)
	if err != nil {
		cxtLog.Error("GetResource Error:", err.Error())
		http.Error(w, "资源id="+resourceid+"对应记录获取错误"+err.Error(), http.StatusNotFound)
		return
	}
	if resource == nil {
		cxtLog.Error("Resource not found")
		http.Error(w, "资源id="+resourceid+"对应记录不存在", http.StatusNotFound)
		return
	}

	cxtLog = cxtLog.WithField("containerID", resource.ContainerID)

	if resource.Status != resourcing.Avaiable {
		cxtLog.WithField("Status", resource.Status).Error("Resource不可用")
		http.Error(w, "资源id="+resourceid+"对应的资源不可用，状态="+resource.Status+"，请确保客户端在Connect状态下进行操作", http.StatusForbidden)
		return
	}

	req.URL, err = url.ParseRequestURI(a.dUrl)
	if err != nil {
		cxtLog.WithField("a.dUrl", a.dUrl).Error("ParseRequestUIR Error:", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var segments []string = strings.Split(req.RequestURI, "/")
	if len(segments) >= 3 {
		segments[1] = "containers"
		segments[2] = resource.ContainerID
	}

	req.RequestURI = strings.Join(segments, "/")

	cxtLog.Info("REDIRECT ", req.Method, ": ", req.RequestURI)
	a.fwd.ServeHTTP(w, req)
}

func (a *Api) redirectToContainerHijack(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceid = data["name"]

	var cxtLog = log.WithField("resourceId", resourceid)

	var resource, err = a.manager.GetResource(resourceid)
	if err != nil {
		cxtLog.Error("GetResource Error :", err.Error())
		http.Error(w, "资源id="+resourceid+"对应记录获取错误"+err.Error(), http.StatusNotFound)
		return
	}
	if resource == nil {
		cxtLog.Error("Resource Not Found")
		http.Error(w, "资源id="+resourceid+"对应记录不存在", http.StatusNotFound)
		return
	}

	cxtLog = cxtLog.WithField("containerID", resource.ContainerID)

	if resource.Status != resourcing.Avaiable {
		cxtLog.Error("资源不可用,Status =", resource.Status)
		http.Error(w, "资源id="+resourceid+"对应的资源不可用，状态="+resource.Status+"，请确保客户端在Connect状态下进行操作", http.StatusForbidden)
		return
	}

	req.RequestURI = replacePath(req.RequestURI, resource.ContainerID)

	req.URL.Path = replacePath(req.URL.Path, resource.ContainerID)

	//cxtLog.Info("req.URL.Path->",req.URL.Path)

	cxtLog.Info("REDIRECT ", req.Method, ": ", req.RequestURI)
	a.swarmHijack(nil, a.dUrl, w, req)
}

func replacePath(path string, ctId string) string {
	var segments []string = strings.Split(path, "/")
	if len(segments) >= 3 {
		segments[1] = "containers"
		segments[2] = ctId
	}
	return strings.Join(segments, "/")
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

func (a *Api) _createContainerByImage(swarm *swarmclient.SwarmClient, imageName string, creatingConfig *dockerclient.ContainerConfig) (string, error) {
	var cxtLog = log.WithField("imageName", imageName)

	cxtLog.Info("Now ,Create Container by image")

	creatingConfig.Image = imageName

	containerid, err := swarm.CreateContainer(creatingConfig, "", nil)
	if err != nil {
		cxtLog.Info("swarm.CreateContainer Error:", err.Error())
		return "", err
	}
	log.Info("Create Success,containerId = ", containerid)

	return containerid, nil
}

func (a *Api) deleteContainer(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceid = data["name"]

	var cxtLog = log.WithField("resourceId", resourceid)

	req.ParseForm()
	v, force := false, false

	if req.FormValue("v") == "1" || strings.ToLower(req.FormValue("v")) == "true" {
		v = true
	}
	if req.FormValue("force") == "1" || strings.ToLower(req.FormValue("force")) == "true" {
		force = true
	}

	var resource, err = a.manager.GetResource(resourceid)

	if err != nil {
		cxtLog.Error("GetResource Error :", err.Error())
		http.Error(w, "资源id="+resourceid+"对应记录获取错误"+err.Error(), http.StatusBadRequest)
		return
	}
	if resource == nil {
		cxtLog.Error("Resource Not Fount")
		http.Error(w, "资源id="+resourceid+"对应记录不存在", http.StatusNotFound)
		return
	}

	cxtLog = cxtLog.WithField("containerID", resource.ContainerID)

	if resource.Status == resourcing.Moving {
		cxtLog.Warn("Status is moving ,cannot be deleted!")
		w.WriteHeader(http.StatusAccepted) //如果是正在移动中该条数据不能立即删除!!
		return
	}

	client, err := a._getSwarmClient()
	if err != nil {
		cxtLog.Error("_getSwarmClient Error :", err.Error())
		http.Error(w, "a._getSwarmClient() Error : "+err.Error(), http.StatusInternalServerError)
		return
	}

	cxtLog.Info("now, removing container")
	//先删除容器
	err1 := client.RemoveContainer(resource.ContainerID, force, v)
	
	if err1 != nil {
		if strings.Contains(err1.Error(),"no such id") || strings.Contains(err1.Error(),"not found") {// docker daemon -> no such id, swarm ->not found
			cxtLog.Error("container seems not exist,  client.RemoveContainer force=", force, " v=", v, " Error: ", err1.Error())
			goto DELETEDB
		}
		
		cxtLog.Error("client.RemoveContainer force=", force, " v=", v, " Error: ", err1.Error())
		http.Error(w, "容器id="+resource.ContainerID+"删除失败: "+err1.Error(), http.StatusInternalServerError)
		return
	}

DELETEDB:
	cxtLog.Info("now, removing resource db record")
	//再删除资源
	err2 := a.manager.DeleteResource(resourceid)

	if err2 != nil {
		cxtLog.Error("DeleteResource Error, resourceId =", resourceid, " Error :", err2.Error())
		http.Error(w, "资源id="+resourceid+"删除失败: "+err2.Error(), http.StatusInternalServerError)
		return
	}

	cxtLog.Info("Delete Success")
	w.WriteHeader(http.StatusNoContent)
	return
}

var (
	movingIdsCache      []string
	movingLocker        sync.RWMutex
	movingProgressCache = map[string][]*movingProgressInfo{} //<resourceid,[]*movingProgressInfo>

)

type movingProgressInfo struct {
	Time time.Time
	Msg  string
	Code string // moving ,end , avaiable,invalide,error
}

//func (p *movingProgressInfo) String() {
//	var bts, err = json.Marshal(p)
//	if err != nil {
//		return "序列化错误" + err.Error()
//	} else {
//		return string(bts)
//	}
//}

//containers/xxxx/move?target=xxx
func (a *Api) moveResource(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceId = data["id"]

	var cxtLog = log.WithField("resourceId", resourceId).WithField("@Method", "webide_api.go/moveResource")

	if resourceId == "" {
		cxtLog.Error("request parameter resourceId should not be empty")
		http.Error(w, fmt.Sprint("id参数不能为空"), http.StatusBadRequest) //moving ,wait
		return
	}

	req.ParseForm()
	var addr = req.FormValue("target")
	if addr == "" {
		cxtLog.Error("request parameter target should not be empty")
		http.Error(w, fmt.Sprint("target参数不能为空"), http.StatusBadRequest)
		return
	}

	//客户端连续多个同一请求同时来时，对 cache的写操作和判定应加锁。
	//防止因为前一个请求判断通过，还没有append进去时，同 一个id另外一个请求又再次通过请求。
	movingLocker.Lock()

	var doing = false

	for _, id := range movingIdsCache {
		if id == resourceId {
			doing = true
		}
	}

	if doing {
		cxtLog.Warn(resourceId, " is moving , please wait")
		http.Error(w, "moving ,please wait", http.StatusAccepted) //moving ,wait
		movingLocker.Unlock()
		return
	}
	movingIdsCache = append(movingIdsCache, resourceId)

	if _, ok := movingProgressCache[resourceId]; ok == true {
		delete(movingProgressCache, resourceId) //删除旧的进度信息
	}

	movingLocker.Unlock()

	cxtLog.Infoln("Moving Resource ", resourceId)

	var resource, err = a.manager.GetResource(resourceId)
	if err != nil {
		cxtLog.Error("GetResource resouceId=", resourceId, "Error :", err.Error())
		http.Error(w, fmt.Sprintf("a.manager.GetResource(%s) Error %s", resourceId, err.Error()), http.StatusInternalServerError) //moving ,wait
		return
	}

	if resource == nil {
		cxtLog.Error("Resource Not Fount")
		http.Error(w, fmt.Sprintf("%s 对应resource不存在", resourceId), http.StatusNotFound) //moving ,wait
		return
	}

	if resource.Status == resourcing.Moving {
		cxtLog.Warnf("检测到不一致数据。%s 缓存中无Moving，DB处于Moving状态。", resourceId)
		http.Error(w, fmt.Sprintf("检测到不一致数据。%s 缓存中无Moving，DB处于Moving状态。", resourceId), http.StatusInternalServerError) //moving ,wait
		return
	}

	endCh, progressCh, errorCh := a._moveResourceAndUpdateDb(resource, addr)

	go monitorProgress(resourceId, progressCh, endCh, errorCh)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{
			msg:"moving"
		}`))
	return
}

//监控处理进度, 将 resourceId相关的处理信息收集到内存中
func monitorProgress(resourceId string, progressCh chan string, endCh chan string, errorCh chan error) {
	var cxtLog = log.WithField("resourceId", resourceId)

	var pCache, ok = movingProgressCache[resourceId]
	if ok == false {
		movingProgressCache[resourceId] = []*movingProgressInfo{}
		pCache = movingProgressCache[resourceId]
	}

	for {
		select {
		case msg := <-progressCh:
			{
				pCache = append(pCache, &movingProgressInfo{Time: time.Now(), Msg: msg, Code: "moving"})
				movingProgressCache[resourceId] = pCache
			}
		case err := <-errorCh:
			{
				cxtLog.Infoln("Moving Progress SIGNAL -> ERROR,resourceid=", resourceId, " Error:", err.Error())
				pCache = append(pCache, &movingProgressInfo{Time: time.Now(), Msg: err.Error(), Code: "error"})
				movingProgressCache[resourceId] = pCache
				goto END
			}
		case <-endCh:
			{
				cxtLog.Infoln("Moving Progress SIGNAL -> END,resourceid=", resourceId)
				pCache = append(pCache, &movingProgressInfo{Time: time.Now(), Msg: "资源移动已完成", Code: "end"})
				movingProgressCache[resourceId] = pCache
				goto END
			}
		}
	}

END:
	go func() {
		log.Infof("资源%s Moving结束, 2分钟后,删除内存中的进度信息", resourceId)
		old := movingProgressCache[resourceId]
		<-time.Tick(2 * time.Minute)                                                                                  //进度信息完成后暂存2分钟 这样可以让客户端在移动结束后,仍然能得到全部的进度信息
		now := movingProgressCache[resourceId]                                                                        //防止在2分钟内，资源又被移动，处于移动中。
		if _, ok := movingProgressCache[resourceId]; ok == true && fmt.Sprintf("%p", old) == fmt.Sprintf("%p", now) { //查看进度信息是否更新了。
			delete(movingProgressCache, resourceId)
			log.Infof("资源%s Moving结束, 进度信息已删除", resourceId)
		}
	}()

	log.Infoln(movingProgressCache[resourceId])

	return
}

//获取 资源移动的进度
func (a *Api) movingProgress(w http.ResponseWriter, req *http.Request) {
	var data = mux.Vars(req)
	var resourceId = data["id"]
	var cxtLog = log.WithField("resourceId", resourceId).WithField("@Method", "movingProgress")
	infoes, ok := movingProgressCache[resourceId]
	w.Header().Set("Content-Type", "application/json")
	//不存在
	if ok == false {
		cxtLog.Info("progress is not in cache.")
		dbResource, err := a.manager.GetResource(resourceId)
		if err != nil {
			cxtLog.Error()
			http.Error(w, fmt.Sprintf("a.manager.GetResource(%s)", resourceId)+err.Error(), http.StatusInternalServerError)
			return
		}

		if dbResource == nil {
			cxtLog.Error("resource not found")
			http.Error(w, fmt.Sprintf("%s资源不存在", resourceId), http.StatusNotFound)
			return
		}

		cxtLog.Info("Status=", dbResource.Status)

		if dbResource.Status == resourcing.Avaiable {
			w.WriteHeader(200)
			var info = []*movingProgressInfo{
				&movingProgressInfo{
					Code: "avaiable", Msg: "资源目前可用,未处于移动状态", Time: time.Now(),
				}}
			data, err := json.Marshal(info)
			if err != nil {
				cxtLog.Error("jsonMarshal Error: ", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			w.Write([]byte(data))
			return
		}

		if dbResource.Status == resourcing.Moving {
			w.WriteHeader(200)
			var info = []*movingProgressInfo{
				&movingProgressInfo{
					Code: "moving", Msg: "无法获取移动进度", Time: time.Now(),
				}}
			data, err := json.Marshal(info)
			if err != nil {
				cxtLog.Info("json.Marshall Error :", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			w.Write([]byte(data))
			return
		}

		if dbResource.Status == resourcing.Image {
			w.WriteHeader(200)
			var info = []*movingProgressInfo{
				&movingProgressInfo{
					Code: "invalide", Msg: "当前资源已镜像化", Time: time.Now(),
				}}
			data, err := json.Marshal(info)
			if err != nil {
				cxtLog.Error("json.Marshall Error :", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			w.Write([]byte(data))
			return
		}
	} else {
		w.WriteHeader(200)
		data, err := json.Marshal(infoes)
		if err != nil {
			cxtLog.Error("json.Marshal Error:", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		w.Write([]byte(data))
		return
	}
}

var requestTimeout = 10 * time.Second

func (a *Api) _moveResourceAndUpdateDb(resource *resourcing.ContainerResource, toAddr string) (newIdCh chan string, progressCh chan string, errorCh chan error) {
	newIdCh = make(chan string)
	errorCh = make(chan error)
	progressCh = make(chan string) //如果progress 缓冲区个数不为1,那么要确保再接受到end的时候，flush一下 progress.

	var cxtLog = log.WithField("toAddr", toAddr).WithField("containerId", resource.ContainerID).WithField("@Method", "_moveResourceAndUpdateDb")

	go func() {

		defer func() {
			//查看最终资源状态，无论移动是否成功，最终都应该置为可用状态
			if resource.Status != resourcing.Avaiable {
				resource.Status = resourcing.Avaiable
				err := a.manager.UpdateResource(resource.ResourceID, resource)
				if err != nil {
					cxtLog.Warn("资源状态更新为可用时失败, UpdateResource Error :", err.Error())
				}
			}
			//无论成功与否，均移除缓存内的标记
			removeMovingIdInCache(resource.ResourceID)
		}()

		progressCh <- "开始移动资源"
		cxtLog.Infoln("开始移动资源")

		resource.Status = resourcing.Moving
		a.manager.UpdateResource(resource.ResourceID, resource)

		client, err := a._getSwarmClient()

		progressCh <- "正在打包提交镜像"
		cxtLog.Infoln("正在打包提交镜像")
		err = client.StopContainer(resource.ContainerID, 60)
		if err != nil {
			progressCh <- "提交前停止容器出现错误： " + err.Error()
			cxtLog.Error("提交前停止容器出现错误： " + err.Error())
			errorCh <- err
			return
		}

		var repo = a.registryAddr + "/webide-moving/" + resource.CreatingConfig.Hostname
		var tag = fmt.Sprintf("%d", time.Now().Unix())
		var coment = fmt.Sprintf("%s Moving 产生临时镜像", time.Now().String())
		id, err := client.Commit(resource.ContainerID, nil, repo, tag, coment, "webide-moving", true)
		cxtLog.Info("Moving 产生临时镜像 image id = ", id)
		if err != nil {
			progressCh <- "提交镜像出现错误： " + err.Error()
			cxtLog.Error("提交镜像出现错误： " + err.Error())
			errorCh <- err
			return
		}
		progressCh <- "镜像打包完成，正在推送镜像"
		cxtLog.Info("镜像打包完成，正在推送镜像  ", repo+":"+tag)
		var imgFullName = repo + ":" + tag
		err = client.PushImage(repo, tag, nil)
		if err != nil {
			progressCh <- "推送镜像时出现错误： " + err.Error()
			cxtLog.Error("推送镜像时出现错误： " + err.Error())
			errorCh <- err
			return
		}

		var config = resource.CreatingConfig
		config.Image = imgFullName
		nodeName, err := a.getNodeNameByNodeAddress(toAddr)
		if err != nil {
			progressCh <- "查找目标服务器时出现错误 "
			cxtLog.Error("查找目标服务器时出现错误： " + err.Error())
			errorCh <- err
			return
		}
		if config.Labels == nil {
			config.Labels = map[string]string{}
		}
		progressCh <- "推送完成，正在重建资源"
		cxtLog.Info("推送完成，正在重建资源")
		config.Labels["com.docker.swarm.constraints"] = fmt.Sprintf(`["node==%s"]`, nodeName) //指定目标服务器
		newId, err := client.CreateContainer(config, "", nil)
		if err != nil {
			progressCh <- "重建资源时出现错误：" + err.Error()
			cxtLog.Errorf("resource %s Moving Fail.  Image Push Success , image full name is  %s , But Create Fail. %s", resource.ResourceID, config.Image, err.Error())
			errorCh <- err
			return
		}

		err = client.StartContainer(newId, nil)
		if err != nil {
			cxtLog.Error("资源创建成功，但无法启动。", err.Error())
			progressCh <- "资源创建成功，但无法启动。" + err.Error()
			errorCh <- errors.New("资源创建成功，但无法启动。" + err.Error())
			return
		}

		progressCh <- "清理旧容器，并更新数据库"
		log.Infoln("正在清理旧容器，并更新数据库")
		err = client.RemoveContainer(resource.ContainerID, true, true)
		if err != nil {
			cxtLog.Warn("移动资源后，清理旧容器出现错误，containerid = " + resource.ContainerID)
		}

		progressCh <- "资源重建成功，正在更新数据库"
		cxtLog.Infoln("资源重建成功，正在更新数据库")
		resource.ContainerID = newId
		resource.Status = resourcing.Avaiable

		err = a.manager.UpdateResource(resource.ResourceID, resource)
		if err != nil {
			errorCh <- err
			progressCh <- "资源重建后更新数据库标识出错，Error : " + err.Error()
			log.Error("资源重建后更新数据库标识出错，Error : " + err.Error())
			return
		}

		progressCh <- "数据库更新完成，资源移动成功"
		cxtLog.Infoln("移动操作成功,返回新的容器id为 ", newId)
		newIdCh <- newId

		return
	}()

	return newIdCh, progressCh, errorCh
}

func removeMovingIdInCache(id string) {
	//禁止读写
	movingLocker.Lock()
	defer movingLocker.Unlock()

	var result = []string{}
	for _, item := range movingIdsCache {
		if item != id {
			result = append(result, item)
		}
	}

	movingIdsCache = result
	return
}

func (a *Api) getNodeNameByNodeAddress(addr string) (name string, err error) {
	var cxtLog = log.WithField("@Method", "getNodeNameByNodeAddress").WithField("addr", addr)

	nodes, err := a.manager.Nodes()
	if err != nil {
		cxtLog.Error("a.manager.Nodes() Error :", err.Error())
		return "", err
	}
	for _, n := range nodes {
		if n.Addr == addr {
			return n.Name, nil
		}
	}

	cxtLog.Error("cannot find node name by addr")
	return "", errors.New(fmt.Sprintf("没有找到%s的node name", addr))

}
