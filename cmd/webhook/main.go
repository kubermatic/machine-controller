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

package main

import (
	"flag"

	"github.com/kubermatic/machine-controller/pkg/admission"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	"github.com/kubermatic/machine-controller/pkg/node"
	userdatamanager "github.com/kubermatic/machine-controller/pkg/userdata/manager"
	osmv1alpha1 "k8c.io/operating-system-manager/pkg/crd/osm/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	masterURL              string
	kubeconfig             string
	admissionListenAddress string
	admissionTLSCertPath   string
	admissionTLSKeyPath    string
	caBundleFile           string
	useOSM                 bool
	namespace              string
)

func main() {
	nodeFlags := node.NewFlags(flag.CommandLine)

	klog.InitFlags(nil)
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if flag.Lookup("master") == nil {
		flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	flag.StringVar(&admissionListenAddress, "listen-address", ":9876", "The address on which the MutatingWebhook will listen on")
	flag.StringVar(&admissionTLSCertPath, "tls-cert-path", "/tmp/cert/cert.pem", "The path of the TLS cert for the MutatingWebhook")
	flag.StringVar(&admissionTLSKeyPath, "tls-key-path", "/tmp/cert/key.pem", "The path of the TLS key for the MutatingWebhook")
	flag.StringVar(&caBundleFile, "ca-bundle", "", "path to a file containing all PEM-encoded CA certificates (will be used instead of the host's certificates if set)")
	flag.StringVar(&namespace, "namespace", "kubermatic", "The namespace where the webhooks will run")

	// OSM specific flags
	flag.BoolVar(&useOSM, "use-osm", false, "osm controller is enabled for node bootstrap")

	flag.Parse()
	kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	if caBundleFile != "" {
		if err := util.SetCABundleFile(caBundleFile); err != nil {
			klog.Fatalf("-ca-bundle is invalid: %v", err)
		}
	}

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig: %v", err)
	}

	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		klog.Fatalf("failed to add corev1 api to scheme: %v", err)
	}

	if err := osmv1alpha1.AddToScheme(scheme); err != nil {
		klog.Fatalf("failed to add osmv1alpha1 api to scheme: %v", err)
	}

	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		klog.Fatalf("failed to build client: %v", err)
	}

	um, err := userdatamanager.New()
	if err != nil {
		klog.Fatalf("error initialising userdata plugins: %v", err)
	}

	srv, err := admission.New(admissionListenAddress, client, um, nodeFlags, useOSM, namespace)
	if err != nil {
		klog.Fatalf("failed to create admission hook: %v", err)
	}

	if err := srv.ListenAndServeTLS(admissionTLSCertPath, admissionTLSKeyPath); err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := srv.Close(); err != nil {
			klog.Fatalf("Failed to shutdown server: %v", err)
		}
	}()
	klog.Infof("Listening on %s", admissionListenAddress)
	select {}
}
