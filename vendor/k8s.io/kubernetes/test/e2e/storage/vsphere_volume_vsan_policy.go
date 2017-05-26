/*
Copyright 2017 The Kubernetes Authors.

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

package storage

import (
	"fmt"
	"time"

	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stype "k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	vsphere "k8s.io/kubernetes/pkg/cloudprovider/providers/vsphere"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	VmfsDatastore                              = "sharedVmfs-0"
	VsanDatastore                              = "vsanDatastore"
	Datastore                                  = "datastore"
	Policy_DiskStripes                         = "diskStripes"
	Policy_HostFailuresToTolerate              = "hostFailuresToTolerate"
	Policy_CacheReservation                    = "cacheReservation"
	Policy_ObjectSpaceReservation              = "objectSpaceReservation"
	Policy_IopsLimit                           = "iopsLimit"
	HostFailuresToTolerateCapabilityVal        = "0"
	CacheReservationCapabilityVal              = "20"
	DiskStripesCapabilityVal                   = "1"
	ObjectSpaceReservationCapabilityVal        = "30"
	IopsLimitCapabilityVal                     = "100"
	StripeWidthCapabilityVal                   = "2"
	DiskStripesCapabilityInvalidVal            = "14"
	HostFailuresToTolerateCapabilityInvalidVal = "4"
)

/*
   Test to verify if VSAN storage capabilities specified in storage-class is being honored while volume creation.
   Valid VSAN storage capabilities are mentioned below:
   1. hostFailuresToTolerate
   2. forceProvisioning
   3. cacheReservation
   4. diskStripes
   5. objectSpaceReservation
   6. iopsLimit

   Steps
   1. Create StorageClass with VSAN storage capabilities set to valid values.
   2. Create PVC which uses the StorageClass created in step 1.
   3. Wait for PV to be provisioned.
   4. Wait for PVC's status to become Bound
   5. Create pod using PVC on specific node.
   6. Wait for Disk to be attached to the node.
   7. Delete pod and Wait for Volume Disk to be detached from the Node.
   8. Delete PVC, PV and Storage Class
*/

var _ = framework.KubeDescribe("VSAN policy support for dynamic provisioning [Volume]", func() {
	f := framework.NewDefaultFramework("volume-vsan-policy")
	var (
		client       clientset.Interface
		namespace    string
		scParameters map[string]string
	)
	BeforeEach(func() {
		framework.SkipUnlessProviderIs("vsphere")
		client = f.ClientSet
		namespace = f.Namespace.Name
		scParameters = make(map[string]string)
		nodeList := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
		if !(len(nodeList.Items) > 0) {
			framework.Failf("Unable to find ready and schedulable Node")
		}
	})

	// Valid policy.
	It("verify VSAN storage capability with valid hostFailuresToTolerate and cacheReservation values is honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy hostFailuresToTolerate: %s, cacheReservation: %s", HostFailuresToTolerateCapabilityVal, CacheReservationCapabilityVal))
		scParameters[Policy_HostFailuresToTolerate] = HostFailuresToTolerateCapabilityVal
		scParameters[Policy_CacheReservation] = CacheReservationCapabilityVal
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		invokeValidVSANPolicyTest(f, client, namespace, scParameters)
	})

	// Valid policy.
	It("verify VSAN storage capability with valid diskStripes and objectSpaceReservation values is honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy diskStripes: %s, objectSpaceReservation: %s", DiskStripesCapabilityVal, ObjectSpaceReservationCapabilityVal))
		scParameters[Policy_DiskStripes] = "1"
		scParameters[Policy_ObjectSpaceReservation] = "30"
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		invokeValidVSANPolicyTest(f, client, namespace, scParameters)
	})

	// Valid policy.
	It("verify VSAN storage capability with valid diskStripes and objectSpaceReservation values and a VSAN datastore is honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy diskStripes: %s, objectSpaceReservation: %s", DiskStripesCapabilityVal, ObjectSpaceReservationCapabilityVal))
		scParameters[Policy_DiskStripes] = DiskStripesCapabilityVal
		scParameters[Policy_ObjectSpaceReservation] = ObjectSpaceReservationCapabilityVal
		scParameters[Datastore] = VsanDatastore
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		invokeValidVSANPolicyTest(f, client, namespace, scParameters)
	})

	// Valid policy.
	It("verify VSAN storage capability with valid objectSpaceReservation and iopsLimit values is honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy objectSpaceReservation: %s, iopsLimit: %s", ObjectSpaceReservationCapabilityVal, IopsLimitCapabilityVal))
		scParameters[Policy_ObjectSpaceReservation] = ObjectSpaceReservationCapabilityVal
		scParameters[Policy_IopsLimit] = IopsLimitCapabilityVal
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		invokeValidVSANPolicyTest(f, client, namespace, scParameters)
	})

	// Invalid VSAN storage capabilties parameters.
	It("verify VSAN storage capability with invalid capability name objectSpaceReserve is not honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy objectSpaceReserve: %s, stripeWidth: %s", ObjectSpaceReservationCapabilityVal, StripeWidthCapabilityVal))
		scParameters["objectSpaceReserve"] = ObjectSpaceReservationCapabilityVal
		scParameters[Policy_DiskStripes] = StripeWidthCapabilityVal
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		err := invokeInvalidVSANPolicyTestNeg(client, namespace, scParameters)
		// This will make sure err is always returned.
		Expect(err).To(HaveOccurred())
		errorMsg := "invalid option \\\"objectSpaceReserve\\\" for diskStripes in volume plugin kubernetes.io/vsphere-volume."
		if !strings.Contains(err.Error(), errorMsg) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	// Invalid policy on a VSAN test bed.
	// diskStripes value has to be between 1 and 12.
	It("verify VSAN storage capability with invalid diskStripes value is not honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy diskStripes: %s, cacheReservation: %s", DiskStripesCapabilityInvalidVal, CacheReservationCapabilityVal))
		scParameters[Policy_DiskStripes] = DiskStripesCapabilityInvalidVal
		scParameters[Policy_CacheReservation] = CacheReservationCapabilityVal
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		err := invokeInvalidVSANPolicyTestNeg(client, namespace, scParameters)
		// This will make sure err is always returned.
		Expect(err).To(HaveOccurred())
		errorMsg := "Invalid value for " + Policy_DiskStripes + " in volume plugin kubernetes.io/vsphere-volume."
		if !strings.Contains(err.Error(), errorMsg) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	// Invalid policy on a VSAN test bed.
	// hostFailuresToTolerate value has to be between 0 and 3 including.
	It("verify VSAN storage capability with invalid hostFailuresToTolerate value is not honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy hostFailuresToTolerate: %s", HostFailuresToTolerateCapabilityInvalidVal))
		scParameters[Policy_HostFailuresToTolerate] = HostFailuresToTolerateCapabilityInvalidVal
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		err := invokeInvalidVSANPolicyTestNeg(client, namespace, scParameters)
		// This will make sure err is always returned.
		Expect(err).To(HaveOccurred())
		errorMsg := "Invalid value for " + Policy_HostFailuresToTolerate + " in volume plugin kubernetes.io/vsphere-volume."
		if !strings.Contains(err.Error(), errorMsg) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	// Specify a valid VSAN policy on a non-VSAN test bed.
	// The test should fail.
	It("verify VSAN storage capability with non-vsan datastore is not honored for dynamically provisioned pvc using storageclass", func() {
		By(fmt.Sprintf("Invoking Test for VSAN policy diskStripes: %s, objectSpaceReservation: %s and a non-VSAN datastore: %s", DiskStripesCapabilityVal, ObjectSpaceReservationCapabilityVal, VmfsDatastore))
		scParameters[Policy_DiskStripes] = DiskStripesCapabilityVal
		scParameters[Policy_ObjectSpaceReservation] = ObjectSpaceReservationCapabilityVal
		scParameters[Datastore] = VmfsDatastore
		framework.Logf("Invoking Test for VSAN storage capabilities: %+v", scParameters)
		err := invokeInvalidVSANPolicyTestNeg(client, namespace, scParameters)
		// This will make sure err is always returned.
		Expect(err).To(HaveOccurred())
		errorMsg := "The specified datastore: \\\"" + VmfsDatastore + "\\\" is not a VSAN datastore. " +
			"The policy parameters will work only with VSAN Datastore."
		framework.Logf("errorMsg: %+q", errorMsg)
		if !strings.Contains(err.Error(), errorMsg) {
			Expect(err).NotTo(HaveOccurred())
		}
	})
})

func invokeValidVSANPolicyTest(f *framework.Framework, client clientset.Interface, namespace string, scParameters map[string]string) {
	By("Creating Storage Class With VSAN policy params")
	storageclass, err := client.StorageV1().StorageClasses().Create(getVSphereStorageClassSpec("vsanpolicysc", scParameters))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create storage class with err: %v", err))
	defer client.StorageV1().StorageClasses().Delete(storageclass.Name, nil)

	By("Creating PVC using the Storage Class")
	pvclaim, err := framework.CreatePVC(client, namespace, getVSphereClaimSpecWithStorageClassAnnotation(namespace, storageclass))
	Expect(err).NotTo(HaveOccurred())
	defer framework.DeletePersistentVolumeClaim(client, pvclaim.Name, namespace)

	pvclaims := make([]*v1.PersistentVolumeClaim, 1)
	pvclaims = append(pvclaims, pvclaim)
	By("Waiting for claim to be in bound phase")
	persistentvolumes, err := framework.WaitForPVClaimBoundPhase(client, pvclaims)
	Expect(err).NotTo(HaveOccurred())

	By("Creating pod to attach PV to the node")
	// Create pod to attach Volume to Node
	pod, err := framework.CreatePod(client, namespace, pvclaims, false, "")
	Expect(err).NotTo(HaveOccurred())

	vsp, err := vsphere.GetVSphere()
	Expect(err).NotTo(HaveOccurred())
	By("Verify the volume is accessible and available in the pod")
	verifyVSphereVolumesAccessible(pod, persistentvolumes, vsp)

	By("Deleting pod")
	framework.DeletePodWithWait(f, client, pod)

	By("Waiting for volumes to be detached from the node")
	waitForVSphereDiskToDetach(vsp, persistentvolumes[0].Spec.VsphereVolume.VolumePath, k8stype.NodeName(pod.Spec.NodeName))
}

func invokeInvalidVSANPolicyTestNeg(client clientset.Interface, namespace string, scParameters map[string]string) error {
	By("Creating Storage Class With VSAN policy params")
	storageclass, err := client.StorageV1().StorageClasses().Create(getVSphereStorageClassSpec("vsanpolicysc", scParameters))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create storage class with err: %v", err))
	defer client.StorageV1().StorageClasses().Delete(storageclass.Name, nil)

	By("Creating PVC using the Storage Class")
	pvclaim, err := framework.CreatePVC(client, namespace, getVSphereClaimSpecWithStorageClassAnnotation(namespace, storageclass))
	Expect(err).NotTo(HaveOccurred())
	defer framework.DeletePersistentVolumeClaim(client, pvclaim.Name, namespace)

	By("Waiting for claim to be in bound phase")
	err = framework.WaitForPersistentVolumeClaimPhase(v1.ClaimBound, client, pvclaim.Namespace, pvclaim.Name, framework.Poll, 2*time.Minute)
	Expect(err).To(HaveOccurred())

	eventList, err := client.CoreV1().Events(pvclaim.Namespace).List(metav1.ListOptions{})
	return fmt.Errorf("Failure message: %+q", eventList.Items[0].Message)
}
