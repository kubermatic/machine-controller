package nodecsrapprover

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	certificatesv1beta1client "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const ControllerName = "node_csr_autoapprover"

// Check if the Reconciler fullfills the interface
// at compile time
var _ reconcile.Reconciler = &reconciler{}

type reconciler struct {
	client.Client
	// Have to use the typed client because csr approval is a subresource
	// the dynamic client does not approve
	certClient certificatesv1beta1client.CertificateSigningRequestInterface
}

func Add(mgr manager.Manager) error {
	certClient, err := certificatesv1beta1client.NewForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("failed to create certificate client: %v", err)
	}

	r := &reconciler{Client: mgr.GetClient(), certClient: certClient.CertificateSigningRequests()}
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed to construct controller: %v", err)
	}
	return c.Watch(&source.Kind{Type: &certificatesv1beta1.CertificateSigningRequest{}}, &handler.EnqueueRequestForObject{})
}

func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := r.reconcile(ctx, request)
	if err != nil {
		klog.Errorf("Reconciliation of request %s failed: %v", request.NamespacedName.String(), err)
	}
	return reconcile.Result{}, err
}

var allowedUsages = []certificatesv1beta1.KeyUsage{certificatesv1beta1.UsageDigitalSignature,
	certificatesv1beta1.UsageKeyEncipherment,
	certificatesv1beta1.UsageServerAuth}

func (r *reconciler) reconcile(ctx context.Context, request reconcile.Request) error {
	// Get the CSR object
	csr := &certificatesv1beta1.CertificateSigningRequest{}
	if err := r.Get(ctx, request.NamespacedName, csr); err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	// If CSR is approved, skip it
	for _, condition := range csr.Status.Conditions {
		if condition.Type == certificatesv1beta1.CertificateApproved {
			klog.V(4).Infoln("already approved, skipping reconciling")
			return nil
		}
	}

	klog.V(4).Infof("Reconciling CSR %s", csr.ObjectMeta.Name)

	// Ensure system:nodes is in groups
	if !sets.NewString(csr.Spec.Groups...).Has("system:nodes") {
		klog.V(4).Infof("Skipping reconciling CSR '%s' because 'system:nodes' is not in its groups", csr.ObjectMeta.Name)
		return nil
	}

	// Check are present usages matching allowed usages
	if len(csr.Spec.Usages) != 3 {
		klog.V(4).Infof("Skipping reconciling CSR '%s' because it has not exactly three usages defined", csr.ObjectMeta.Name)
		return nil
	}
	for _, usage := range csr.Spec.Usages {
		if !isUsageInUsageList(usage, allowedUsages) {
			klog.V(4).Infof("Skipping reconciling CSR '%s' because its usage (%v) is not in the list of allowed usages (%v)",
				csr.ObjectMeta.Name, usage, allowedUsages)
			return nil
		}
	}
	// Validate the CSR request
	if err := validateCSRRequest(csr); err != nil {
		klog.V(4).Infof("Skipping reconciling CSR '%s' because CSR request is not valid: %v", csr.ObjectMeta.Name, err)
		return nil
	}

	klog.V(4).Infof("Approving CSR %s", csr.ObjectMeta.Name)
	approvalCondition := certificatesv1beta1.CertificateSigningRequestCondition{
		Type:   certificatesv1beta1.CertificateApproved,
		Reason: "machine-controller NodeCSRApprover controller approved node serving cert",
	}
	csr.Status.Conditions = append(csr.Status.Conditions, approvalCondition)

	if _, err := r.certClient.UpdateApproval(csr); err != nil {
		return fmt.Errorf("failed to update approval for CSR %q: %v", csr.Name, err)
	}

	klog.Infof("Successfully approved CSR %s", csr.ObjectMeta.Name)
	return nil
}

func isUsageInUsageList(usage certificatesv1beta1.KeyUsage, usageList []certificatesv1beta1.KeyUsage) bool {
	for _, usageListItem := range usageList {
		if usage == usageListItem {
			return true
		}
	}
	return false
}

// validateCSRRequest parses the CSR request and compares certificate request CommonName with CSR username, and
// certificateRequest Organization with CSR groups.
func validateCSRRequest(csr *certificatesv1beta1.CertificateSigningRequest) error {
	csrBlock, rest := pem.Decode(csr.Spec.Request)
	if csrBlock == nil {
		return fmt.Errorf("no certificate request found for the given CSR")
	}
	if len(rest) != 0 {
		return fmt.Errorf("found more than one PEM encoded block in the result")
	}
	csrRequest, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		return err
	}
	if csrRequest.Subject.CommonName != csr.Spec.Username {
		return fmt.Errorf("commonName '%s' is diffrent then CSR username '%s'", csrRequest.Subject.CommonName, csr.Spec.Username)
	}
	for _, org := range csrRequest.Subject.Organization {
		if !sets.NewString(csr.Spec.Groups...).Has(org) {
			return fmt.Errorf("organization '%s' is not in CSR groups", org)
		}
	}
	return nil
}
