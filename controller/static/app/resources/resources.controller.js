(function () {
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
        vm.selectedResource = null;
        vm.selectedResourceId = "";
        vm.newName = "";

        vm.showDeleteResourceDialog = showDeleteResourceDialog;
        vm.showMoveResourceDialog=showMoveResourceDialog;
        vm.moveResource=moveResource;

        vm.deleteResource = deleteResource;
        vm.refresh = refresh;
        vm.containerStatusText = containerStatusText;
        vm.checkAll = checkAll;
        vm.clearAll = clearAll;
        vm.deleteAll = deleteAll;
        vm.nodeName = nodeName;
        vm.containerName = containerName;

        refresh();

        // Apply jQuery to dropdowns in table once ngRepeat has finished rendering
        $scope.$on('ngRepeatFinished', function () {
            $('.ui.sortable.celled.table').tablesort();
            $('#select-all-table-header').unbind();
            $('.ui.right.pointing.dropdown').dropdown();
        });

        $('#multi-action-menu').sidebar({ dimPage: false, animation: 'overlay', transition: 'overlay' });

        $scope.$watch(function () {
            var count = 0;
            angular.forEach(vm.selected, function (s) {
                if (s.Selected) {
                    count += 1;
                }
            });
            vm.selectedItemCount = count;
        });

        // Remove selected items that are no longer visible 
        $scope.$watchCollection('filteredResources', function () {
            angular.forEach(vm.selected, function (s) {
                if (vm.selected[s.ResourceID].Selected == true) {
                    var isVisible = false
                    angular.forEach($scope.filteredResources, function (c) {
                        if (c.ResourceID == s.ResourceID) {
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
                if (s.Selected == true) {
                    ResourceService.delete(s.ResourceID)
                        .then(function (data) {
                            delete vm.selected[s.ResourceID];
                            vm.refresh();
                        }, function (data) {
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
                .then(function (data) {
                    vm.resources = data;
                    angular.forEach(vm.resources, function (resource) {
                        vm.selected[resource.ResourceID] = { ResourceID: resource.ResourceID, Selected: vm.selectedAll };
                    });
                }, function (data) {
                    vm.error = data;
                });

            ResourceService.nodes()
                .then(function (data) {
                    vm.nodes = data
                }, function (err) {
                    vm.error = err
                })

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
        
        function showMoveResourceDialog(resource){
            vm.selectedResourceId= resource.ResourceID;
            $('#move-modal').modal('show');
        }

        function deleteResource() {
            ResourceService.delete(vm.selectedResourceId)
                .then(function (data) {
                    vm.refresh();
                }, function (data) {
                    vm.error = data;
                });
        }
        
        function moveResource(){
            var resourceId=vm.selectedResourceId;
            var addr =vm.selectedNode;
            if(addr=="" || addr== undefined){
                alert("请选择目标服务器");
                return;
            }
            ResourceService.move(resourceId,addr)
            .then(function(data){
                vm.refresh();
            },function(data){
                vm.error=data;
            });
        }


        function containerStatusText(container) {
            if (container == undefined) {
                return "-"
            }

            if (container.Status.indexOf("Up") == 0) {
                if (container.Status.indexOf("(Paused)") != -1) {
                    return "Paused";
                }
                return "Running";
            }
            else if (container.Status.indexOf("Exited") == 0) {
                return "Stopped";
            }
            return "Unknown";
        }

        function nodeName(container) {
            if (container == undefined) {
                return "-"
            }
            // Return only the node name (first component of the shortest container name)
            var components = shortestContainerName(container).split('/');
            return components[1];
        }
        function containerName(container) {
            // Remove the node name by returning the last name component of the shortest container name
            var components = shortestContainerName(container).split('/');
            return components[components.length - 1];
        }
        function shortestContainerName(container) {
            // Distill shortest container name by taking the name with the fewest components
            // Names with the same number of components are considered in undefined order
            var shortestName = "";
            var minComponents = 99;

            var names = container.Names
            for (var i = 0; i < names.length; i++) {
                var name = names[i];
                var numComponents = name.split('/').length
                if (numComponents < minComponents) {
                    shortestName = name;
                    minComponents = numComponents;
                }
            }

            return shortestName;
        }
    }
})();
