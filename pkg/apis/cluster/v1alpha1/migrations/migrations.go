/*
Copyright 2019 The Machine Controller Authors.

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

package migrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	machinecontrolleradmission "github.com/kubermatic/machine-controller/pkg/admission"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1/conversions"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	"github.com/kubermatic/machine-controller/pkg/machines"
	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	dynamicclient "k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func MigrateProviderConfigToProviderSpecIfNecessary(ctx context.Context, log *zap.SugaredLogger, config *restclient.Config, client ctrlruntimeclient.Client) error {
	log.Info("Starting to migrate providerConfigs to providerSpecs")
	dynamicClient, err := dynamicclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to construct dynamic client: %w", err)
	}

	machineGVR := schema.GroupVersionResource{Group: "cluster.k8s.io", Version: "v1alpha1", Resource: "machines"}
	machineSetGVR := schema.GroupVersionResource{Group: "cluster.k8s.io", Version: "v1alpha1", Resource: "machinesets"}
	machineDeploymentsGVR := schema.GroupVersionResource{Group: "cluster.k8s.io", Version: "v1alpha1", Resource: "machinedeployments"}

	machines, err := dynamicClient.Resource(machineGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list machine objects: %w", err)
	}
	for _, machine := range machines.Items {
		machineLog := log.With("machine", ctrlruntimeclient.ObjectKeyFromObject(&machine))

		marshalledObject, err := machine.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal unstructured machine %s: %w", machine.GetName(), err)
		}
		convertedMachine, wasConverted, err := conversions.Convert_Machine_ProviderConfig_To_ProviderSpec(marshalledObject)
		if err != nil {
			return fmt.Errorf("failed to convert machine: %w", err)
		}
		if wasConverted {
			machineLog.Info("Converted providerConfig -> providerSpec, attempting to update")
			if convertedMachine.Annotations == nil {
				convertedMachine.Annotations = map[string]string{}
			}
			// We must set this, otherwise the webhook will deny our update request because modifications to a machines
			// spec are not allowed
			convertedMachine.Annotations[machinecontrolleradmission.BypassSpecNoModificationRequirementAnnotation] = "true"
			if err := client.Update(ctx, convertedMachine); err != nil {
				return fmt.Errorf("failed to update converted machine %s: %w", convertedMachine.Name, err)
			}
			machineLog.Info("Successfully updated machine after converting providerConfig -> providerSpec")
		}
	}

	machineSets, err := dynamicClient.Resource(machineSetGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list MachineSets: %w", err)
	}
	for _, machineSet := range machineSets.Items {
		machineSetLog := log.With("machineset", ctrlruntimeclient.ObjectKeyFromObject(&machineSet))

		marshalledObject, err := machineSet.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal unstructured MachineSet %s: %w", machineSet.GetName(), err)
		}
		convertedMachineSet, machineSetWasConverted, err := conversions.Convert_MachineSet_ProviderConfig_To_ProviderSpec(marshalledObject)
		if err != nil {
			return fmt.Errorf("failed to convert MachineSet %s/%s: %w", machineSet.GetNamespace(), machineSet.GetName(), err)
		}
		if machineSetWasConverted {
			machineSetLog.Info("Converted providerConfig -> providerSpec, attempting to update")
			if err := client.Update(ctx, convertedMachineSet); err != nil {
				return fmt.Errorf("failed to update MachineSet %s/%s after converting providerConfig -> providerSpec: %w", convertedMachineSet.Namespace, convertedMachineSet.Name, err)
			}
			machineSetLog.Info("Successfully updated MachineSet after converting providerConfig -> providerSpec")
		}
	}

	machineDeployments, err := dynamicClient.Resource(machineDeploymentsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list MachineDeplyoments: %w", err)
	}
	for _, machineDeployment := range machineDeployments.Items {
		machineDeploymentLog := log.With("machinedeployment", ctrlruntimeclient.ObjectKeyFromObject(&machineDeployment))

		marshalledObject, err := machineDeployment.MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to marshal unstructured MachineDeployment %s: %w", machineDeployment.GetName(), err)
		}
		convertedMachineDeployment, machineDeploymentWasConverted, err := conversions.Convert_MachineDeployment_ProviderConfig_To_ProviderSpec(marshalledObject)
		if err != nil {
			return fmt.Errorf("failed to convert MachineDeployment %s/%s: %w", machineDeployment.GetNamespace(), machineDeployment.GetName(), err)
		}
		if machineDeploymentWasConverted {
			machineDeploymentLog.Info("Converted providerConfig -> providerSpec, attempting to update")
			if err := client.Update(ctx, convertedMachineDeployment); err != nil {
				return fmt.Errorf("failed to update MachineDeployment %s/%s after converting providerConfig -> providerSpec: %w", convertedMachineDeployment.Namespace, convertedMachineDeployment.Name, err)
			}
			machineDeploymentLog.Info("Successfully updated MachineDeployment after converting providerConfig -> providerSpec")
		}
	}

	log.Info("Successfully migrated providerConfigs to providerSpecs")
	return nil
}

func MigrateMachinesv1Alpha1MachineToClusterv1Alpha1MachineIfNecessary(
	ctx context.Context, log *zap.SugaredLogger,
	client ctrlruntimeclient.Client,
	kubeClient kubernetes.Interface,
	providerData *cloudprovidertypes.ProviderData) error {
	var (
		cachePopulatingInterval = 15 * time.Second
		cachePopulatingTimeout  = 10 * time.Minute
		noMigrationNeed         = false
	)

	crdLog := log.With("crd", machines.CRDName)

	err := wait.Poll(cachePopulatingInterval, cachePopulatingTimeout, func() (done bool, err error) {
		err = client.Get(ctx, types.NamespacedName{Name: machines.CRDName}, &apiextensionsv1.CustomResourceDefinition{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				noMigrationNeed = true
				return true, nil
			}

			var cerr *cache.ErrCacheNotStarted
			if errors.As(err, &cerr) {
				log.Info("Cache hasn't started yet, trying in 5 seconds")
				return false, nil
			}

			return false, fmt.Errorf("failed to get crds: %w", err)
		}
		return true, nil
	})

	if err != nil {
		crdLog.Errorw("Failed waiting for caches to be populated", zap.Error(err))
		return err
	}

	if noMigrationNeed {
		crdLog.Info("CRD not present, no migration needed")
		return nil
	}

	err = client.Get(ctx, types.NamespacedName{Name: "machines.cluster.k8s.io"}, &apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		return fmt.Errorf("error when checking for existence of 'machines.cluster.k8s.io' crd: %w", err)
	}

	if err := migrateMachines(ctx, log, client, kubeClient, providerData); err != nil {
		return fmt.Errorf("failed to migrate machines: %w", err)
	}
	crdLog.Info("Attempting to delete CRD")
	if err := client.Delete(ctx, &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: machines.CRDName}}); err != nil {
		return fmt.Errorf("failed to delete machinesv1alpha1.machine crd: %w", err)
	}
	crdLog.Info("Successfully deleted CRD")
	return nil
}

func migrateMachines(ctx context.Context, log *zap.SugaredLogger, client ctrlruntimeclient.Client, kubeClient kubernetes.Interface, providerData *cloudprovidertypes.ProviderData) error {
	log.Info("Starting migration for machine.machines.k8s.io/v1alpha1 to machine.cluster.k8s.io/v1alpha1")

	// Get machinesv1Alpha1Machines
	log.Info("Getting existing machine.machines.k8s.io/v1alpha1 to migrate")

	machinesv1Alpha1Machines := &machinesv1alpha1.MachineList{}
	if err := client.List(ctx, machinesv1Alpha1Machines); err != nil {
		return fmt.Errorf("failed to list machinesV1Alpha1 machines: %w", err)
	}
	log.Infof("Found %d machine.machines.k8s.io/v1alpha1 resources", len(machinesv1Alpha1Machines.Items))

	// Convert the machine, create the new machine, delete the old one, wait for it to be absent
	// We do this in one loop to avoid ending up having all machines in  both the new and the old format if deletion
	// fails for whatever reason
	for _, machinesV1Alpha1Machine := range machinesv1Alpha1Machines.Items {
		machineLog := log.With("machine", machinesV1Alpha1Machine.Name)
		machineLog.Info("Starting migration")

		convertedClusterv1alpha1Machine := &clusterv1alpha1.Machine{}
		err := conversions.Convert_MachinesV1alpha1Machine_To_ClusterV1alpha1Machine(&machinesV1Alpha1Machine,
			convertedClusterv1alpha1Machine)
		if err != nil {
			return fmt.Errorf("failed to convert machinesV1alpha1.machine to clusterV1alpha1.machine name=%s err=%w",
				machinesV1Alpha1Machine.Name, err)
		}
		convertedClusterv1alpha1Machine.Finalizers = append(convertedClusterv1alpha1Machine.Finalizers, machinecontroller.FinalizerDeleteNode)

		// Some providers need to update the provider instance to the new UID, we get the provider as early as possible
		// to not fail in a half-migrated state when the providerconfig is invalid
		providerConfig, err := providerconfigtypes.GetConfig(convertedClusterv1alpha1Machine.Spec.ProviderSpec)
		if err != nil {
			return fmt.Errorf("failed to get provider config: %w", err)
		}
		skg := providerconfig.NewConfigVarResolver(ctx, client)
		prov, err := cloudprovider.ForProvider(providerConfig.CloudProvider, skg)
		if err != nil {
			return fmt.Errorf("failed to get cloud provider %q: %w", providerConfig.CloudProvider, err)
		}

		machineLog = machineLog.With("provider", providerConfig.CloudProvider)

		// We will set that to what's finally in the apisever, be that a created a clusterv1alpha1machine
		// or a preexisting one, because the migration got interrupted
		// It is required to set the ownerRef of the node
		var finalClusterV1Alpha1Machine *clusterv1alpha1.Machine

		// Do a get first to cover the case the new machine was already created but then something went wrong
		// If that is the case and the clusterv1alpha1machine != machinesv1alpha1machine we error out and the operator
		// has to manually delete either the new or the old machine
		machineLog = machineLog.With("converted", ctrlruntimeclient.ObjectKeyFromObject(convertedClusterv1alpha1Machine))
		machineLog.Info("Checking if converted machine already exists")

		existingClusterV1alpha1Machine := &clusterv1alpha1.Machine{}
		err = client.Get(ctx,
			types.NamespacedName{Namespace: convertedClusterv1alpha1Machine.Namespace, Name: convertedClusterv1alpha1Machine.Name},
			existingClusterV1alpha1Machine)
		if err != nil {
			// Some random error occurred
			if !kerrors.IsNotFound(err) {
				return fmt.Errorf("failed to check if converted machine %s already exists: %w", convertedClusterv1alpha1Machine.Name, err)
			}

			// ClusterV1alpha1Machine does not exist yet
			machineLog.Info("Converted machine does not yet exist, attempting to create it")

			if err := client.Create(ctx, convertedClusterv1alpha1Machine); err != nil {
				return fmt.Errorf("failed to create clusterv1alpha1.machine %s: %w", convertedClusterv1alpha1Machine.Name, err)
			}
			machineLog.Info("Successfully created converted machine")
			finalClusterV1Alpha1Machine = convertedClusterv1alpha1Machine
		} else {
			// ClusterV1alpha1Machine already exists
			if !equality.Semantic.DeepEqual(convertedClusterv1alpha1Machine.Spec, existingClusterV1alpha1Machine.Spec) {
				return fmt.Errorf("---manual intervention required!--- Spec of machines.v1alpha1.machine %s is not equal to clusterv1alpha1.machines %s/%s, delete either of them to allow migration to succeed",
					machinesV1Alpha1Machine.Name, convertedClusterv1alpha1Machine.Namespace, convertedClusterv1alpha1Machine.Name)
			}
			existingClusterV1alpha1Machine.Labels = convertedClusterv1alpha1Machine.Labels
			existingClusterV1alpha1Machine.Annotations = convertedClusterv1alpha1Machine.Annotations
			existingClusterV1alpha1Machine.Finalizers = convertedClusterv1alpha1Machine.Finalizers

			machineLog.Info("Updating existing converted machine")

			if err := client.Update(ctx, existingClusterV1alpha1Machine); err != nil {
				return fmt.Errorf("failed to update metadata of existing clusterV1Alpha1 machine: %w", err)
			}
			machineLog.Info("Successfully updated existing converted machine")
			finalClusterV1Alpha1Machine = existingClusterV1alpha1Machine
		}

		// We have to ensure there is an ownerRef to our clusterv1alpha1.Machine on the node if it exists
		// and that there is no ownerRef to the old machine anymore
		if err := ensureClusterV1Alpha1NodeOwnership(ctx, machineLog, finalClusterV1Alpha1Machine, client); err != nil {
			return err
		}

		if sets.NewString(finalClusterV1Alpha1Machine.Finalizers...).Has(machinecontroller.FinalizerDeleteInstance) {
			machineLog.Info("Attempting to update the UID at the cloud provider")
			newMachineWithOldUID := finalClusterV1Alpha1Machine.DeepCopy()
			newMachineWithOldUID.UID = machinesV1Alpha1Machine.UID
			if err := prov.MigrateUID(ctx, machineLog, newMachineWithOldUID, finalClusterV1Alpha1Machine.UID); err != nil {
				return fmt.Errorf("running the provider migration for the UID failed: %w", err)
			}
			// Block until we can actually GET the instance with the new UID
			var isMigrated bool
			for i := 0; i < 100; i++ {
				if _, err := prov.Get(ctx, machineLog, finalClusterV1Alpha1Machine, providerData); err == nil {
					isMigrated = true
					break
				}
				time.Sleep(10 * time.Second)
			}
			if !isMigrated {
				return fmt.Errorf("failed to GET instance for machine %s after UID migration", finalClusterV1Alpha1Machine.Name)
			}
			machineLog.Info("Successfully updated the UID at the cloud provider")
		}

		// All went fine, we only have to clear the old machine now
		machineLog.Info("Deleting old machine object")
		if err := deleteMachinesV1Alpha1Machine(ctx, &machinesV1Alpha1Machine, client); err != nil {
			return err
		}
		machineLog.Info("Successfully deleted old machine object")
		machineLog.Info("Successfully finished migration")
	}

	log.Info("Successfully finished migration for machine.machines.k8s.io/v1alpha1 to machine.cluster.k8s.io/v1alpha1")
	return nil
}

func ensureClusterV1Alpha1NodeOwnership(ctx context.Context, machineLog *zap.SugaredLogger, machine *clusterv1alpha1.Machine, client ctrlruntimeclient.Client) error {
	if machine.Spec.Name == "" {
		machine.Spec.Name = machine.Name
	}
	machineLog.Info("Checking if node for machines exists")
	nodeNameCandidates := []string{machine.Spec.Name}
	if machine.Status.NodeRef != nil {
		if machine.Status.NodeRef.Name != machine.Spec.Name {
			nodeNameCandidates = append(nodeNameCandidates, machine.Status.NodeRef.Name)
		}
	}

	for _, nodeName := range nodeNameCandidates {
		node := &corev1.Node{}
		if err := client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			if kerrors.IsNotFound(err) {
				machineLog.Info("No node for machines found")
				continue
			}
			return fmt.Errorf("Failed to get node %s for machine %s: %w",
				machine.Spec.Name, machine.Name, err)
		}

		nodeLog := machineLog.With("node", node.Name)
		nodeLog.Info("Found node for machine, removing its ownerRef and adding NodeOwnerLabel")

		nodeLabels := node.Labels
		nodeLabels[machinecontroller.NodeOwnerLabelName] = string(machine.UID)
		// We retry this because nodes get frequently updated so there is a reasonable chance this may fail
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
				return err
			}
			// Clear all OwnerReferences as a safety measure
			node.OwnerReferences = nil
			node.Labels = nodeLabels
			return client.Update(ctx, node)
		}); err != nil {
			return fmt.Errorf("failed to update OwnerLabel on node %s: %w", node.Name, err)
		}
		nodeLog.Info("Successfully removed ownerRef and added NodeOwnerLabelName to node")
	}

	return nil
}

func deleteMachinesV1Alpha1Machine(ctx context.Context,
	machine *machinesv1alpha1.Machine, client ctrlruntimeclient.Client) error {
	machine.Finalizers = []string{}
	if err := client.Update(ctx, machine); err != nil {
		return fmt.Errorf("failed to update machinesv1alpha1.machine %s after removing finalizer: %w", machine.Name, err)
	}
	if err := client.Delete(ctx, machine); err != nil {
		return fmt.Errorf("failed to delete machine %s: %w", machine.Name, err)
	}

	if err := wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		return isMachinesV1Alpha1MachineDeleted(ctx, machine.Name, client)
	}); err != nil {
		return fmt.Errorf("failed to wait for machine %s to be deleted: %w", machine.Name, err)
	}

	return nil
}

func isMachinesV1Alpha1MachineDeleted(ctx context.Context, name string, client ctrlruntimeclient.Client) (bool, error) {
	if err := client.Get(ctx, types.NamespacedName{Name: name}, &machinesv1alpha1.Machine{}); err != nil {
		if kerrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
