package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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

func (a *Api) initWebIDEDB() {

}

//create 之后 start并且返回详细的Container信息，同时插入数据库 相应的数据
func (a *Api) applyContainer(w http.ResponseWriter, req *http.Request) {
	var query = req.URL.RawQuery
	a.doRequest(req.Method, req.RequestURI, req.Body, req.Header)

}

func (a *Api) connectContainer(w http.ResponseWriter, req *http.Request) {
	_ := w, req
}

func (a *APi) abandonContainer(w http.ResponseWriter, req *http.Request) {
	_ := w
	_ := req
}
func (a *APi) statusContainer(w http.ResponseWriter, req *http.Request) {
	_ := w, req
}

func (a *Api) doRequest(method string, url string, body io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	if headers != nil {
		for header, value := range headers {
			req.Header.Add(header, value)
		}
	}

	resp, err := http.Client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, error.Error("Not Found")
	}
	if resp.StatusCode >= 400 {
		return nil, Error{StatusCode: resp.StatusCode, Status: resp.Status, msg: string(data)}
	}

	return data, nil

}
