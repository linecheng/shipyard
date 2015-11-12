(function(){
    'use strict';

    angular
        .module('shipyard.resources')
        .controller('ResourcesController', ResourcesController);

    ResourcesController.$inject = ['$scope', 'ResourceService', '$state'];
    function ResourcesController($scope, ResourceService, $state) {
        var vm = this;
        
        vm.error = "";
        vm.errors = [];
        vm.resources = [];
        vm.selected = {};
        vm.selectedItemCount = 0;
        vm.selectedAll = false;
        vm.numOfInstances = 1;
        vm.selectedResource= null;
        vm.selectedResourceId = "";
        vm.newName = "";

        vm.showDeleteResourceDialog = showDeleteResourceDialog;
        
        vm.deleteResource = deleteResource;
        vm.refresh = refresh;
        vm.resourceStatusText = resourceStatusText;
        vm.checkAll = checkAll;
        vm.clearAll = clearAll;
        vm.deleteAll = deleteAll;

        refresh();

        // Apply jQuery to dropdowns in table once ngRepeat has finished rendering
        $scope.$on('ngRepeatFinished', function() {
            $('.ui.sortable.celled.table').tablesort();
            $('#select-all-table-header').unbind();
            $('.ui.right.pointing.dropdown').dropdown();
        });

        $('#multi-action-menu').sidebar({dimPage: false, animation: 'overlay', transition: 'overlay'});

        $scope.$watch(function() {
            var count = 0;
            angular.forEach(vm.selected, function (s) {
                if(s.Selected) {
                    count += 1;
                }
            });
            vm.selectedItemCount = count;
        });

        // Remove selected items that are no longer visible 
        $scope.$watchCollection('filteredResources', function () {
            angular.forEach(vm.selected, function(s) {
                if(vm.selected[s.ResourceID].Selected == true) {
                    var isVisible = false
                    angular.forEach($scope.filteredResources, function(c) {
                        if(c.ResourceID == s.ResourceID) {
                            isVisible = true;
                            return;
                        }
                    });
                    vm.selected[s.ResourceID].Selected = isVisible;
                }
            });
            return;
        });

        function clearAll() {
            angular.forEach(vm.selected, function (s) {
                vm.selected[s.ResourceID].Selected = false;
            });
        }

        function deleteAll() {
            angular.forEach(vm.selected, function (s) {
                if(s.Selected == true) {
                    ResourceService.delete(s.ResourceID)
                        .then(function(data) {
                            delete vm.selected[s.ResourceID];
                            vm.refresh();
                        }, function(data) {
                            vm.error = data;
                        });
                }
            });
        }

        function checkAll() {
            angular.forEach($scope.filteredResources, function (resource) {
                vm.selected[resource.ResourceID].Selected = vm.selectedAll;
            });
        }

        function refresh() {
            ResourceService.list()
                .then(function(data) {
                    vm.resources = data; 
                    angular.forEach(vm.resources, function (resource) {
                        vm.selected[resource.ResourceID] = {ResourceID: resource.ResourceID, Selected: vm.selectedAll};
                    });
                }, function(data) {
                    vm.error = data;
                });

            vm.error = "";
            vm.errors = [];
            vm.resources = [];
            vm.selected = {};
            vm.selectedItemCount = 0;
            vm.selectedAll = false;
            vm.numOfInstances = 1;
            vm.selectedResourceId = "";
            vm.newName = "";
        }

        function showDeleteResourceDialog(resource) {
            vm.selectedResourceId = resource.ResourceID;
            $('#delete-modal').modal('show');
        }

        function deleteResource() {
            ResourceService.delete(vm.selectedResourceId)
                .then(function(data) {
                    vm.refresh();
                }, function(data) {
                    vm.error = data;
                });
        }


        function resourceStatusText(resource) {
           return resource.status;
        }  
    }
})();
