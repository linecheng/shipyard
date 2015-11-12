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
                var promise = $http
                    .get('/resources/list')
                    .then(function(response) {
                        return response.data;
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
