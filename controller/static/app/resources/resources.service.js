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
                        	var rList = response.data;
						
		var p =$http.get("/containers/json?all=1")				
		.then(function(response){
			var cts = response.data;
			var cListIDs =cts.map(item=>item.Id);
			var data = rList.map(item=>{
				item.ExistContainer = cListIDs.indexOf(item.ContainerID)>=0;
				var time = new Date(item.CreateTime);
				item.CreateTime=time.getFullYear()+"-"+time.getMonth()+"-"+time.getDate()+"   " +time.getHours()+":"+time.getMinutes()+":"+time.getSeconds();
				return item;
			});
			return data;
		});
		
		return p;
                    });

//	var promise = Promise.all([getResList,cts]).then(function(value){
//	    var rList = value[0];
//	    var cListIDs =value[1].map(item=>item.Id);
//	    var data = rList.map(item=>{
//			item.ExistContainer = cListIDs.indexOf(item.ContainerID)>=0;
//			var time = new Date(item.CreateTime);
//			item.CreateTime=time.getFullYear()+"-"+time.getMonth()+"-"+time.getDate()+"   " +time.getHours()+":"+time.getMinutes()+":"+time.getSeconds();
//			return item;
//		});
//	    return data;
//	});
	
                return getResList;
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
