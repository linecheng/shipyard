package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	dockerclient "github.com/samalba/dockerclient"
	cring "github.com/shipyard/shipyard/containerresourcing"
	"github.com/shipyard/shipyard/swarmclient"
	"github.com/shipyard/shipyard/tools/CRConsistent/config"
	"github.com/shipyard/shipyard/tools/CRConsistent/dbaccess"
	"github.com/spf13/viper"
)

var (
	dbAccess *dbaccess.DBAccess
	swarmUrl string
)

func main() {

	var err error
	config.InitConfig()
	swarmUrl = viper.GetString("swarm.url")
	log.Infoln("swarm url = ", swarmUrl)
	debugLevel := viper.GetInt("debug-level")
	log.SetLevel(log.Level(debugLevel))
	dbAccess, err = dbaccess.New()
	if err != nil {
		log.Fatalln("dbaccess.New() 错误 :", err.Error())
		return
	}
	var now = time.Now().Format("[ 2006-01-02 15:04:05 ]")
	log.Infoln(now + "Wait Tick .......")
	var lastOp time.Duration = 0 * time.Nanosecond
	var tickTime = 1*time.Minute + lastOp

	for _ = range time.Tick(tickTime) {
		//var begin = time.Now()

		now = time.Now().Format("[ 2006-01-02 15:04:05 ]")
		log.Infoln(now + "TICK===================================")
		var completeCh = make(chan bool)
		err = run(completeCh)
		if err != nil {
			log.Errorln("container resource consistent error : ", err.Error())
			//		continue
		}

		<-completeCh
		now = time.Now().Format("[ 2006-01-02 15:04:05 ]")
		log.Infoln(now + "TICK END ===============================")
		log.Infoln("wait next tick")
		time.Sleep(5 * time.Second)
		//		var end = time.Now()

		//		lastOp = end.Sub(begin)
		//		tickTime = 1*time.Minute + lastOp
	}

}

func run(completeCh chan bool) (err error) {

	defer func() {
		if err != nil {
			completeCh <- true
		}
	}()

	log.Debugln("in run")
	noResource_Containers, noContainer_Resources, err := diffContainersAndResources()
	_ = noContainer_Resources

	if err != nil {
		return err
	}

	client, err := swarmclient.NewSwarmClient(swarmUrl, nil)
	_ = client
	if err != nil {
		return err
	}

	if len(noResource_Containers) <= 0 {
		log.Info(fmt.Sprintf("操作结束,成功删除%d个,失败%d个", 0, 0))
		//为了让管道接受准备好
		go func() {
			time.Sleep(1 * time.Second)
			completeCh <- true
		}()
		return nil
	}

	var failCh = make(chan *dockerclient.Container, len(noResource_Containers))
	var successCh = make(chan *dockerclient.Container, len(noResource_Containers))

	go func(total int) {
		var fail = 0
		var success = 0
		for {
			select {
			case <-failCh:
				fail++
			case <-successCh:
				success++
			}

			if (fail + success) >= total {
				log.Info(fmt.Sprintf("操作结束,成功删除%d个,失败%d个", success, fail))
				completeCh <- true //发送完成标识
				return
			}
		}
	}(len(noResource_Containers))

	//for each to remove
	for i, _ := range noResource_Containers {
		c := noResource_Containers[i]
		go func() {
			err = client.RemoveContainer(c.Id, true, true)
			log.Infoln(c.Id + " [remove]")
			if err != nil {
				log.Infoln(fmt.Sprintf("client.RemoveContainer(%s) Error : %s", c.Id, err.Error()))
				failCh <- c
			} else {
				log.Infoln(fmt.Sprintf("%s Remove Success", c.Id))
				successCh <- c
			}
			//不能在这里进行成功个数失败总个数的检测,非线程安全.  多个协程并行进行,很有可能 fail++过之后,当前协程停止另外一个协程运行到这里,
			//那么当前这个协程 return 提示操作结束了,第一个协程会再次输出当前提示.
		}()
	}

	return nil
}

//func removeContainers(c *dockerclient.Container,rmCount){

//}

//func removeContainersF(total int) {
//	client, err := swarmclient.NewSwarmClient(swarmUrl,nil)
//	if err!=nil{
//		return nil
//	}
//	var success=0
//	var fail =0

//	return func(id,force,v){
//		err = client.RemoveContainer(id,force,v)

//		if err!=nil{
//			fail++
//		}else{
//			success++
//		}

//		if (success+fail==total){
//			log.Info("........")
//		}
//	}
//}

func getContainers() ([]dockerclient.Container, error) {

	client, err := swarmclient.NewSwarmClient(swarmUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("swarmclient.NewSwarmClient(%s,nil) Error :  %s", swarmUrl, err.Error())
	}

	res, err := client.ListContainers(true, true, "")
	if err != nil {
		return nil, fmt.Errorf("client.ListContainers(true,true,''))  swarmUrl=%s   Error :  %v", swarmUrl, err)
	}

	return res, nil
}

func diffContainersAndResources() ([]*dockerclient.Container, []*cring.ContainerResource, error) {
	resources, err := dbAccess.ResourceList()

	if err != nil {
		return nil, nil, errors.New("dbAccess.ResourceList() Error   : " + err.Error())
	}

	log.Infoln("ResourceList :")
	for _, item := range resources {
		log.Infoln(item.ResourceID + "---->" + item.ContainerID)
	}

	containers, err := getContainers()
	if err != nil {
		return nil, nil, fmt.Errorf("getContainers() Error : %v", err)
	}

	log.Infoln("containers : ")

	for _, item := range containers {
		log.Infoln(item.Id)
	}

	var noResource_Containers []*dockerclient.Container
	var noContainer_Resources []*cring.ContainerResource

	var dic = map[*cring.ContainerResource]bool{} //  <*cring.ContainerResource,ishavecontainers>

	for i, _ := range containers {

		var c = &containers[i]
		var referenced = false

		for j, _ := range resources {
			var r = &resources[j]

			if _, ok := dic[r]; ok == false {
				dic[r] = false //第一次遍历时初始化为false
			}

			if c.Id == r.ContainerID {
				referenced = true
				dic[r] = true //当前的Resource 有相应的 Container
			}
		}

		if referenced == false && isInReverse(c) == false {
			noResource_Containers = append(noResource_Containers, c)
		}
	}

	for key, value := range dic {
		if value == false {
			noContainer_Resources = append(noContainer_Resources, key)
		}
	}

	log.Infoln("NOresource containers [will remove] :")
	if len(noResource_Containers) == 0 {
		log.Infoln("[ ]")
	}
	for _, item := range noResource_Containers {
		log.Infoln(item.Id)
	}

	//	log.Infoln("no container resources :")
	//	for _, item := range noContainer_Resources {
	//		log.Infoln(item.ResourceID + "----->" + item.ContainerID)
	//	}

	return noResource_Containers, noContainer_Resources, nil
}

func isInReverse(ct *dockerclient.Container) bool {
	images := viper.GetStringSlice("reverse.image")
	names := viper.GetStringSlice("reverse.name")

	for _, image := range images {
		if image == ct.Image {
			return true
		}
	}

	for _, name := range names {
		var segments = strings.Split(ct.Names[0], "/")
		if name == segments[len(segments)-1] {
			return true
		}
	}

	return false
}
