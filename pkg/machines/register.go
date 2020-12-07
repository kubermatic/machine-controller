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

package machines

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

type resource struct {
	plural string
	kind   string
}

const CRDName = v1alpha1.MachineResourcePlural + "." + v1alpha1.GroupName

var resourceNames = []resource{
	{
		plural: "machines",
		kind:   reflect.TypeOf(v1alpha1.Machine{}).Name(),
	},
}

func EnsureCustomResourceDefinitions(ctx context.Context, clientset apiextensionsclient.Interface) error {
	for _, res := range resourceNames {
		if err := createCustomResourceDefinition(ctx, res.plural, res.kind, clientset); err != nil {
			return err
		}
	}

	return nil
}

func CustomResourceDefinitionExists(ctx context.Context, name string, clientset apiextensionsclient.Interface) (bool, error) {
	crd, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	for _, cond := range crd.Status.Conditions {
		switch cond.Type {
		case apiextensionsv1beta1.Established:
			if cond.Status == apiextensionsv1beta1.ConditionTrue {
				return true, err
			}
		case apiextensionsv1beta1.NamesAccepted:
			if cond.Status == apiextensionsv1beta1.ConditionFalse {
				return false, fmt.Errorf("name conflict: %v", cond.Reason)
			}
		}
	}

	return false, nil
}

func createCustomResourceDefinition(ctx context.Context, plural, kind string, clientset apiextensionsclient.Interface) error {
	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: CRDName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   v1alpha1.GroupName,
			Version: v1alpha1.SchemeGroupVersion.Version,
			Scope:   apiextensionsv1beta1.ClusterScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: plural,
				Kind:   kind,
			},
		},
	}

	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{})
	if err != nil {
		if kerrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}

	// wait for CRD being established
	err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		return CustomResourceDefinitionExists(ctx, CRDName, clientset)
	})
	if err != nil {
		deleteErr := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(ctx, CRDName, metav1.DeleteOptions{})
		if deleteErr != nil {
			return errors.NewAggregate([]error{err, deleteErr})
		}
		return err
	}
	return nil
}
