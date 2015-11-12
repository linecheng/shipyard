(function(){
    'use strict';

    angular
        .module('shipyard.resources')
        .config(getRoutes);

    getRoutes.$inject = ['$stateProvider', '$urlRouterProvider'];

    function getRoutes($stateProvider, $urlRouterProvider) {
        $stateProvider
            .state('dashboard.resources', {
                url: '^/resources',
                templateUrl: 'app/resources/resources.html',
                controller: 'ResourcesController',
                controllerAs: 'vm',
                authenticate: true
            })
        .state('dashboard.deleteR', {
            url: '^/delete',
            templateUrl: 'app/resources/delete.html',
            controller: 'ResourceDeleteController',
            controllerAs: 'vm',
            authenticate: true,
            resolve: {
                containers: ['ResourceService', '$state', '$stateParams', function(ResourceService, $state, $stateParams) {
                    return ResourceService.list().then(null, function(errorData) {
                        $state.go('error');
                    });
                }]
            }
        });
    }
})();
