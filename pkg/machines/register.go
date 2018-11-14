package machines

import (
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

// This is a deliberate change to verify if tests fail
const CRDName = v1alpha1.MachineResourcePlural + "." + v1alpha1.GroupName

var resourceNames = []resource{
	{
		plural: "machines",
		kind:   reflect.TypeOf(v1alpha1.Machine{}).Name(),
	},
}

func EnsureCustomResourceDefinitions(clientset apiextensionsclient.Interface) error {
	for _, res := range resourceNames {
		if err := createCustomResourceDefinition(res.plural, res.kind, clientset); err != nil {
			return err
		}
	}

	return nil
}

func CustomResourceDefinitionExists(name string, clientset apiextensionsclient.Interface) (bool, error) {
	crd, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(name, metav1.GetOptions{})
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
				return false, fmt.Errorf("Name conflict: %v\n", cond.Reason)
			}
		}
	}

	return false, nil
}

func createCustomResourceDefinition(plural, kind string, clientset apiextensionsclient.Interface) error {
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

	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil {
		if kerrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}

	// wait for CRD being established
	err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		return CustomResourceDefinitionExists(CRDName, clientset)
	})
	if err != nil {
		deleteErr := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(CRDName, nil)
		if deleteErr != nil {
			return errors.NewAggregate([]error{err, deleteErr})
		}
		return err
	}
	return nil
}
