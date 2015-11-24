(function(){
    'use strict';

    angular
        .module('shipyard.resources')
        .factory('ResourceService', ResourceService)

        ResourceService.$inject = ['$http'];
     var  prefix ="/origin";
    function ResourceService($http) {
        return {
            list: function() {
                var getResList = $http
                    .get('/resources/list')
                    .then(function(response) {
                        return response.data;
                    });
	var cts = $http.get("/containers/json?all=1")				
		.then(function(response){
		    return response.data;	
		});
	var promise = Promise.all([getResList,cts]).then(function(value){
	    var rList = value[0];
	    var cListIDs =value[1].map(item=>item.Id);
	    var data = rList.map(item=>{
			item.ExistContainer = cListIDs.indexOf(item.ContainerID)>=0
			return item;
		});
	    return data;
	});
	
                return promise;
            },
            delete: function(resourceId) {
                var promise = $http
                    .delete('/resources/' + resourceId)
                    .then(function(response) {
                        return response.data;
                    });
                return promise;
            }
        } 
    }
})();
