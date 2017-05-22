/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package node

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	clientv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	extensionslisters "k8s.io/kubernetes/pkg/client/listers/extensions/v1beta1"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/kubelet/util/format"
	nodepkg "k8s.io/kubernetes/pkg/util/node"
	utilversion "k8s.io/kubernetes/pkg/util/version"

	"github.com/golang/glog"
)

// deletePods will delete all pods from master running on given node, and return true
// if any pods were deleted, or were found pending deletion.
func deletePods(kubeClient clientset.Interface, recorder record.EventRecorder, nodeName, nodeUID string, daemonStore extensionslisters.DaemonSetLister) (bool, error) {
	remaining := false
	selector := fields.OneTermEqualSelector(api.PodHostField, nodeName).String()
	options := metav1.ListOptions{FieldSelector: selector}
	pods, err := kubeClient.Core().Pods(metav1.NamespaceAll).List(options)
	var updateErrList []error

	if err != nil {
		return remaining, err
	}

	if len(pods.Items) > 0 {
		recordNodeEvent(recorder, nodeName, nodeUID, v1.EventTypeNormal, "DeletingAllPods", fmt.Sprintf("Deleting all Pods from Node %v.", nodeName))
	}

	for _, pod := range pods.Items {
		// Defensive check, also needed for tests.
		if pod.Spec.NodeName != nodeName {
			continue
		}

		// Set reason and message in the pod object.
		if _, err = setPodTerminationReason(kubeClient, &pod, nodeName); err != nil {
			if errors.IsConflict(err) {
				updateErrList = append(updateErrList,
					fmt.Errorf("update status failed for pod %q: %v", format.Pod(&pod), err))
				continue
			}
		}
		// if the pod has already been marked for deletion, we still return true that there are remaining pods.
		if pod.DeletionGracePeriodSeconds != nil {
			remaining = true
			continue
		}
		// if the pod is managed by a daemonset, ignore it
		_, err := daemonStore.GetPodDaemonSets(&pod)
		if err == nil { // No error means at least one daemonset was found
			continue
		}

		glog.V(2).Infof("Starting deletion of pod %v/%v", pod.Namespace, pod.Name)
		recorder.Eventf(&pod, v1.EventTypeNormal, "NodeControllerEviction", "Marking for deletion Pod %s from Node %s", pod.Name, nodeName)
		if err := kubeClient.Core().Pods(pod.Namespace).Delete(pod.Name, nil); err != nil {
			return false, err
		}
		remaining = true
	}

	if len(updateErrList) > 0 {
		return false, utilerrors.NewAggregate(updateErrList)
	}
	return remaining, nil
}

// setPodTerminationReason attempts to set a reason and message in the pod status, updates it in the apiserver,
// and returns an error if it encounters one.
func setPodTerminationReason(kubeClient clientset.Interface, pod *v1.Pod, nodeName string) (*v1.Pod, error) {
	if pod.Status.Reason == nodepkg.NodeUnreachablePodReason {
		return pod, nil
	}

	pod.Status.Reason = nodepkg.NodeUnreachablePodReason
	pod.Status.Message = fmt.Sprintf(nodepkg.NodeUnreachablePodMessage, nodeName, pod.Name)

	var updatedPod *v1.Pod
	var err error
	if updatedPod, err = kubeClient.Core().Pods(pod.Namespace).UpdateStatus(pod); err != nil {
		return nil, err
	}
	return updatedPod, nil
}

func forcefullyDeletePod(c clientset.Interface, pod *v1.Pod) error {
	var zero int64
	glog.Infof("NodeController is force deleting Pod: %v:%v", pod.Namespace, pod.Name)
	err := c.Core().Pods(pod.Namespace).Delete(pod.Name, &metav1.DeleteOptions{GracePeriodSeconds: &zero})
	if err == nil {
		glog.V(4).Infof("forceful deletion of %s succeeded", pod.Name)
	}
	return err
}

// forcefullyDeleteNode immediately the node. The pods on the node are cleaned
// up by the podGC.
func forcefullyDeleteNode(kubeClient clientset.Interface, nodeName string) error {
	if err := kubeClient.Core().Nodes().Delete(nodeName, nil); err != nil {
		return fmt.Errorf("unable to delete node %q: %v", nodeName, err)
	}
	return nil
}

// maybeDeleteTerminatingPod non-gracefully deletes pods that are terminating
// that should not be gracefully terminated.
func (nc *NodeController) maybeDeleteTerminatingPod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			glog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		pod, ok = tombstone.Obj.(*v1.Pod)
		if !ok {
			glog.Errorf("Tombstone contained object that is not a Pod %#v", obj)
			return
		}
	}

	// consider only terminating pods
	if pod.DeletionTimestamp == nil {
		return
	}

	node, err := nc.nodeLister.Get(pod.Spec.NodeName)
	// if there is no such node, do nothing and let the podGC clean it up.
	if errors.IsNotFound(err) {
		return
	}
	if err != nil {
		// this can only happen if the Store.KeyFunc has a problem creating
		// a key for the pod. If it happens once, it will happen again so
		// don't bother requeuing the pod.
		utilruntime.HandleError(err)
		return
	}

	// delete terminating pods that have been scheduled on
	// nodes that do not support graceful termination
	// TODO(mikedanese): this can be removed when we no longer
	// guarantee backwards compatibility of master API to kubelets with
	// versions less than 1.1.0
	v, err := utilversion.ParseSemantic(node.Status.NodeInfo.KubeletVersion)
	if err != nil {
		glog.V(0).Infof("Couldn't parse version %q of node: %v", node.Status.NodeInfo.KubeletVersion, err)
		utilruntime.HandleError(nc.forcefullyDeletePod(pod))
		return
	}
	if v.LessThan(gracefulDeletionVersion) {
		utilruntime.HandleError(nc.forcefullyDeletePod(pod))
		return
	}
}

// update ready status of all pods running on given node from master
// return true if success
func markAllPodsNotReady(kubeClient clientset.Interface, node *v1.Node) error {
	// Don't set pods to NotReady if the kubelet is running a version that
	// doesn't understand how to correct readiness.
	// TODO: Remove this check when we no longer guarantee backward compatibility
	// with node versions < 1.2.0.
	if nodeRunningOutdatedKubelet(node) {
		return nil
	}
	nodeName := node.Name
	glog.V(2).Infof("Update ready status of pods on node [%v]", nodeName)
	opts := metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector(api.PodHostField, nodeName).String()}
	pods, err := kubeClient.Core().Pods(metav1.NamespaceAll).List(opts)
	if err != nil {
		return err
	}

	errMsg := []string{}
	for _, pod := range pods.Items {
		// Defensive check, also needed for tests.
		if pod.Spec.NodeName != nodeName {
			continue
		}

		for i, cond := range pod.Status.Conditions {
			if cond.Type == v1.PodReady {
				pod.Status.Conditions[i].Status = v1.ConditionFalse
				glog.V(2).Infof("Updating ready status of pod %v to false", pod.Name)
				_, err := kubeClient.Core().Pods(pod.Namespace).UpdateStatus(&pod)
				if err != nil {
					glog.Warningf("Failed to update status for pod %q: %v", format.Pod(&pod), err)
					errMsg = append(errMsg, fmt.Sprintf("%v", err))
				}
				break
			}
		}
	}
	if len(errMsg) == 0 {
		return nil
	}
	return fmt.Errorf("%v", strings.Join(errMsg, "; "))
}

// nodeRunningOutdatedKubelet returns true if the kubeletVersion reported
// in the nodeInfo of the given node is "outdated", meaning < 1.2.0.
// Older versions were inflexible and modifying pod.Status directly through
// the apiserver would result in unexpected outcomes.
func nodeRunningOutdatedKubelet(node *v1.Node) bool {
	v, err := utilversion.ParseSemantic(node.Status.NodeInfo.KubeletVersion)
	if err != nil {
		glog.Errorf("couldn't parse version %q of node %v", node.Status.NodeInfo.KubeletVersion, err)
		return true
	}
	if v.LessThan(podStatusReconciliationVersion) {
		glog.Infof("Node %v running kubelet at (%v) which is less than the minimum version that allows nodecontroller to mark pods NotReady (%v).", node.Name, v, podStatusReconciliationVersion)
		return true
	}
	return false
}

func nodeExistsInCloudProvider(cloud cloudprovider.Interface, nodeName types.NodeName) (bool, error) {
	instances, ok := cloud.Instances()
	if !ok {
		return false, fmt.Errorf("%v", ErrCloudInstance)
	}
	if _, err := instances.ExternalID(nodeName); err != nil {
		if err == cloudprovider.InstanceNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func recordNodeEvent(recorder record.EventRecorder, nodeName, nodeUID, eventtype, reason, event string) {
	ref := &clientv1.ObjectReference{
		Kind:      "Node",
		Name:      nodeName,
		UID:       types.UID(nodeUID),
		Namespace: "",
	}
	glog.V(2).Infof("Recording %s event message for node %s", event, nodeName)
	recorder.Eventf(ref, eventtype, reason, "Node %s event: %s", nodeName, event)
}

func recordNodeStatusChange(recorder record.EventRecorder, node *v1.Node, new_status string) {
	ref := &clientv1.ObjectReference{
		Kind:      "Node",
		Name:      node.Name,
		UID:       node.UID,
		Namespace: "",
	}
	glog.V(2).Infof("Recording status change %s event message for node %s", new_status, node.Name)
	// TODO: This requires a transaction, either both node status is updated
	// and event is recorded or neither should happen, see issue #6055.
	recorder.Eventf(ref, v1.EventTypeNormal, new_status, "Node %s status is now: %s", node.Name, new_status)
}

// Returns true in case of success and false otherwise
func swapNodeControllerTaint(kubeClient clientset.Interface, taintToAdd, taintToRemove *v1.Taint, node *v1.Node) bool {
	taintToAdd.TimeAdded = metav1.Now()
	err := controller.AddOrUpdateTaintOnNode(kubeClient, node.Name, taintToAdd)
	if err != nil {
		utilruntime.HandleError(
			fmt.Errorf(
				"unable to taint %v unresponsive Node %q: %v",
				taintToAdd.Key,
				node.Name,
				err))
		return false
	}
	glog.V(4).Infof("Added %v Taint to Node %v", taintToAdd, node.Name)

	err = controller.RemoveTaintOffNode(kubeClient, node.Name, taintToRemove, node)
	if err != nil {
		utilruntime.HandleError(
			fmt.Errorf(
				"unable to remove %v unneeded taint from unresponsive Node %q: %v",
				taintToRemove.Key,
				node.Name,
				err))
		return false
	}
	glog.V(4).Infof("Made sure that Node %v has no %v Taint", node.Name, taintToRemove)
	return true
}

func createAddNodeHandler(f func(node *v1.Node) error) func(obj interface{}) {
	return func(originalObj interface{}) {
		obj, err := api.Scheme.DeepCopy(originalObj)
		if err != nil {
			utilruntime.HandleError(err)
			return
		}
		node := obj.(*v1.Node)

		if err := f(node); err != nil {
			utilruntime.HandleError(fmt.Errorf("Error while processing Node Delete: %v", err))
		}
	}
}

func createUpdateNodeHandler(f func(oldNode, newNode *v1.Node) error) func(oldObj, newObj interface{}) {
	return func(origOldObj, origNewObj interface{}) {
		oldObj, err := api.Scheme.DeepCopy(origOldObj)
		if err != nil {
			utilruntime.HandleError(err)
			return
		}
		newObj, err := api.Scheme.DeepCopy(origNewObj)
		if err != nil {
			utilruntime.HandleError(err)
			return
		}
		node := newObj.(*v1.Node)
		prevNode := oldObj.(*v1.Node)

		if err := f(prevNode, node); err != nil {
			utilruntime.HandleError(fmt.Errorf("Error while processing Node Add/Delete: %v", err))
		}
	}
}

func createDeleteNodeHandler(f func(node *v1.Node) error) func(obj interface{}) {
	return func(originalObj interface{}) {
		obj, err := api.Scheme.DeepCopy(originalObj)
		if err != nil {
			utilruntime.HandleError(err)
			return
		}

		node, isNode := obj.(*v1.Node)
		// We can get DeletedFinalStateUnknown instead of *v1.Node here and
		// we need to handle that correctly. #34692
		if !isNode {
			deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
			if !ok {
				glog.Errorf("Received unexpected object: %v", obj)
				return
			}
			node, ok = deletedState.Obj.(*v1.Node)
			if !ok {
				glog.Errorf("DeletedFinalStateUnknown contained non-Node object: %v", deletedState.Obj)
				return
			}
		}

		if err := f(node); err != nil {
			utilruntime.HandleError(fmt.Errorf("Error while processing Node Add/Delete: %v", err))
		}
	}
}
