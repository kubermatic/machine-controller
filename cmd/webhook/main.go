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

	"github.com/Masterminds/semver/v3"

	"github.com/kubermatic/machine-controller/pkg/admission"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	"github.com/kubermatic/machine-controller/pkg/node"
	userdatamanager "github.com/kubermatic/machine-controller/pkg/userdata/manager"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type options struct {
	masterURL               string
	kubeconfig              string
	admissionListenAddress  string
	admissionTLSCertPath    string
	admissionTLSKeyPath     string
	caBundleFile            string
	useOSM                  bool
	useExternalBootstrap    bool
	namespace               string
	workerClusterKubeconfig string
	versionConstraint       string
}

func main() {
	nodeFlags := node.NewFlags(flag.CommandLine)
	opt := &options{}

	klog.InitFlags(nil)
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&opt.kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if flag.Lookup("master") == nil {
		flag.StringVar(&opt.masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	flag.StringVar(&opt.admissionListenAddress, "listen-address", ":9876", "The address on which the MutatingWebhook will listen on")
	flag.StringVar(&opt.admissionTLSCertPath, "tls-cert-path", "/tmp/cert/cert.pem", "The path of the TLS cert for the MutatingWebhook")
	flag.StringVar(&opt.admissionTLSKeyPath, "tls-key-path", "/tmp/cert/key.pem", "The path of the TLS key for the MutatingWebhook")
	flag.StringVar(&opt.caBundleFile, "ca-bundle", "", "path to a file containing all PEM-encoded CA certificates (will be used instead of the host's certificates if set)")
	flag.StringVar(&opt.namespace, "namespace", "kubermatic", "The namespace where the webhooks will run")
	flag.StringVar(&opt.workerClusterKubeconfig, "worker-cluster-kubeconfig", "", "Path to kubeconfig of worker/user cluster where machines and machinedeployments exist. If not specified, value from --kubeconfig or in-cluster config will be used")
	flag.StringVar(&opt.versionConstraint, "kubernetes-version-constraints", ">=0.0.0", "")

	// OSM specific flags
	flag.BoolVar(&opt.useOSM, "use-osm", false, "DEPRECATED: osm controller is enabled for node bootstrap [use use-external-bootstrap instead]")
	flag.BoolVar(&opt.useExternalBootstrap, "use-external-bootstrap", false, "user-data is provided by external bootstrap mechanism (e.g. operating-system-manager, also known as OSM)")

	flag.Parse()
	opt.kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	opt.masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	if opt.caBundleFile != "" {
		if err := util.SetCABundleFile(opt.caBundleFile); err != nil {
			klog.Fatalf("-ca-bundle is invalid: %v", err)
		}
	}

	cfg, err := clientcmd.BuildConfigFromFlags(opt.masterURL, opt.kubeconfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig: %v", err)
	}

	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		klog.Fatalf("failed to build client: %v", err)
	}

	constraint, err := semver.NewConstraint(opt.versionConstraint)
	if err != nil {
		klog.Fatalf("failed to validate kubernetes-version-constraints: %v", err)
	}

	// Start with assuming that current cluster will be used as worker cluster
	workerClient := client
	// Handing for worker client
	if opt.workerClusterKubeconfig != "" {
		workerClusterConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: opt.workerClusterKubeconfig},
			&clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			klog.Fatal(err)
		}

		// Build dedicated client for worker cluster
		workerClient, err = ctrlruntimeclient.New(workerClusterConfig, ctrlruntimeclient.Options{})
		if err != nil {
			klog.Fatalf("failed to build worker client: %v", err)
		}
	}

	um, err := userdatamanager.New()
	if err != nil {
		klog.Fatalf("error initialising userdata plugins: %v", err)
	}

	srv, err := admission.Builder{
		ListenAddress:        opt.admissionListenAddress,
		Client:               client,
		WorkerClient:         workerClient,
		UserdataManager:      um,
		UseExternalBootstrap: opt.useExternalBootstrap || opt.useOSM,
		NodeFlags:            nodeFlags,
		Namespace:            opt.namespace,
		VersionConstraints:   constraint,
	}.Build()
	if err != nil {
		klog.Fatalf("failed to create admission hook: %v", err)
	}

	if err := srv.ListenAndServeTLS(opt.admissionTLSCertPath, opt.admissionTLSKeyPath); err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := srv.Close(); err != nil {
			klog.Fatalf("Failed to shutdown server: %v", err)
		}
	}()
	klog.Infof("Listening on %s", opt.admissionListenAddress)
	select {}
}
