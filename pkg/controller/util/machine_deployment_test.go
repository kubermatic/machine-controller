package util_test

import (
	"testing"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	machinecontroller "k8c.io/machine-controller/pkg/controller/machine"
	"k8c.io/machine-controller/pkg/controller/util"
	clusterv1alpha1 "k8c.io/machine-controller/sdk/apis/cluster/v1alpha1"
)

// TestSetNewMachineSetAnnotationsCopiesEvictionTimeoutOverride covers the
// MachineDeployment leg of the skip-eviction-after override: a
// deployment-level annotation only reaches Machines because
// SetNewMachineSetAnnotations copies it onto the MachineSet object, from
// where the MachineSet controller copies it onto each Machine it creates.
// This is the sibling of TestCreateMachineCopiesEvictionTimeoutOverride,
// which covers the MachineSet-to-Machine leg.
//
// The revision, revision-history, replica and last-applied annotations are
// managed by SetNewMachineSetAnnotations itself and must never be copied
// from the MachineDeployment, so their expected values are asserted per
// case as well. An empty want* string asserts the annotation is absent
// (map lookups return "" for missing keys; none of these annotations has a
// legitimate empty value).
func TestSetNewMachineSetAnnotationsCopiesEvictionTimeoutOverride(t *testing.T) {
	log := zap.NewNop().Sugar()

	tests := []struct {
		name         string
		deployment   *clusterv1alpha1.MachineDeployment
		newMS        *clusterv1alpha1.MachineSet
		newRevision  string
		exists       bool
		wantChanged  bool
		wantOverride string
		wantRevision string
		wantHistory  string
		wantDesired  string
		wantMax      string
	}{
		{
			name: "MachineDeployment eviction timeout override lands on the MachineSet",
			deployment: &clusterv1alpha1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						machinecontroller.AnnotationSkipEvictionAfter: "45m",
					},
				},
				Spec: clusterv1alpha1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(3)),
					// Strategy must be non-nil: MaxSurge dereferences it via
					// IsRollingUpdate. The zero type is not RollingUpdate, so MaxSurge is 0.
					Strategy: &clusterv1alpha1.MachineDeploymentStrategy{},
				},
			},
			newMS: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-ms",
					Namespace: "kubermatic",
				},
			},
			newRevision:  "1",
			exists:       false,
			wantChanged:  true,
			wantOverride: "45m",
			wantRevision: "1",
			wantDesired:  "3",
			wantMax:      "3",
		},
		{
			name: "no override annotation, nothing copied",
			deployment: &clusterv1alpha1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
				},
			},
			newMS: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-ms",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						util.RevisionAnnotation: "2",
					},
				},
			},
			newRevision:  "1",
			exists:       true,
			wantChanged:  false,
			wantRevision: "2",
		},
		{
			// The override must survive the copy while the deployment's own
			// revision/replica values must not: revision and replica
			// annotations on the MachineSet come from newRevision and
			// Spec.Replicas, never from the deployment's annotations.
			name: "revision and replica annotations are not copied from the MachineDeployment",
			deployment: &clusterv1alpha1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						machinecontroller.AnnotationSkipEvictionAfter: "45m",
						util.RevisionAnnotation:                       "7",
						util.RevisionHistoryAnnotation:                "1,2",
						util.DesiredReplicasAnnotation:                "99",
						util.MaxReplicasAnnotation:                    "100",
						corev1.LastAppliedConfigAnnotation:            "{}",
					},
				},
				Spec: clusterv1alpha1.MachineDeploymentSpec{
					Replicas: ptr.To(int32(3)),
					Strategy: &clusterv1alpha1.MachineDeploymentStrategy{},
				},
			},
			newMS: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-ms",
					Namespace: "kubermatic",
				},
			},
			newRevision:  "1",
			exists:       false,
			wantChanged:  true,
			wantOverride: "45m",
			wantRevision: "1",
			wantDesired:  "3",
			wantMax:      "3",
		},
		{
			name: "same override value, no change reported",
			deployment: &clusterv1alpha1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						machinecontroller.AnnotationSkipEvictionAfter: "45m",
					},
				},
			},
			newMS: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-ms",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						machinecontroller.AnnotationSkipEvictionAfter: "45m",
						util.RevisionAnnotation:                       "2",
					},
				},
			},
			newRevision:  "1",
			exists:       true,
			wantChanged:  false,
			wantOverride: "45m",
			wantRevision: "2",
		},
		{
			name: "changed override value propagates",
			deployment: &clusterv1alpha1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						machinecontroller.AnnotationSkipEvictionAfter: "45m",
					},
				},
			},
			newMS: &clusterv1alpha1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-ms",
					Namespace: "kubermatic",
					Annotations: map[string]string{
						machinecontroller.AnnotationSkipEvictionAfter: "30m",
						util.RevisionAnnotation:                       "2",
					},
				},
			},
			newRevision:  "1",
			exists:       true,
			wantChanged:  true,
			wantOverride: "45m",
			wantRevision: "2",
			wantHistory:  "2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := util.SetNewMachineSetAnnotations(log, test.deployment, test.newMS, test.newRevision, test.exists)

			if changed != test.wantChanged {
				t.Errorf("Expected changed=%t, got %t", test.wantChanged, changed)
			}

			assertAnnotation := func(label, key, want string) {
				t.Helper()
				if got := test.newMS.Annotations[key]; got != want {
					t.Errorf("Expected MachineSet %s annotation %q to be %q, got %q", label, key, want, got)
				}
			}
			assertAnnotation("override", machinecontroller.AnnotationSkipEvictionAfter, test.wantOverride)
			assertAnnotation("revision", util.RevisionAnnotation, test.wantRevision)
			assertAnnotation("revision-history", util.RevisionHistoryAnnotation, test.wantHistory)
			assertAnnotation("desired-replicas", util.DesiredReplicasAnnotation, test.wantDesired)
			assertAnnotation("max-replicas", util.MaxReplicasAnnotation, test.wantMax)
			assertAnnotation("last-applied", corev1.LastAppliedConfigAnnotation, "")
		})
	}
}
