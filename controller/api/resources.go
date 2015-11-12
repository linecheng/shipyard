package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
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
