package api

import (
	"encoding/json"
	"net/http"
    log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
    "fmt"
)

func (a *Api) resources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	list, err := a.manager.ResourceList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *Api) resource(w http.ResponseWriter,r *http.Request){
    w.Header().Set("content-type","application/json")
    vars := mux.Vars(r)
    var resourceId=vars["id"]
    if resourceId==""{
        http.Error(w,"resource id 参数缺失",http.StatusBadRequest)
        log.Errorf("400 %s resource id参数缺失",r.RequestURI)
        return
    }
    
    cxtLog := log.WithField("ResourceId",resourceId)
    
    model,err := a.manager.GetResource(resourceId)
    if err!=nil{
        var msg = fmt.Sprintf("Get Resource Error %s",err.Error())
        http.Error(w,msg,http.StatusBadRequest)
        cxtLog.Errorf("400 %s",msg)
    }
    
    if model==nil{
        var msg = fmt.Sprintf("resource id is error, Resource Not Found")
        http.Error(w,msg,http.StatusNotFound)
        cxtLog.Errorf("400 %s",msg)
    }
    
    if err :=json.NewEncoder(w).Encode(model);err!=nil{
		http.Error(w, "json encoder error :" + err.Error(), http.StatusInternalServerError)
        cxtLog.Errorf("500 json encoder error :"+err.Error())
		return
    }
}

func (a *Api) deleteResource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	vars := mux.Vars(r)
	var resourceId = vars["id"]
	if resourceId == "" {
		http.Error(w, "resource id参数缺失", http.StatusBadRequest)
		return
	}
	err := a.manager.DeleteResource(resourceId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	return
}
