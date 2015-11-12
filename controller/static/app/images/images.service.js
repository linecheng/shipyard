(function(){
	'use strict';

	angular
	    .module('shipyard.images')
            .factory('ImagesService', ImagesService);

	ImagesService.$inject = ['$http'];
	var prefix="origin";
        function ImagesService($http) {
            return {
                list: function() {
                    var promise = $http
                        .get(prefix+'/images/json?all=1')
                        .then(function(response) {
                            return response.data;
                        });
                    return promise;
                },
                remove: function(image) {
                    var promise = $http
                        .delete(prefix+'/images/' + image.Id + '?force=1')
                        .then(function(response) {
                            return response.data;
                        });
                    return promise;
                }
            } 
        } 
})();
