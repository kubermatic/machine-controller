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
	"log"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/kubermatic/machine-controller/pkg/admission"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	machinecontrollerlog "github.com/kubermatic/machine-controller/pkg/log"
	"github.com/kubermatic/machine-controller/pkg/node"

	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

type options struct {
	masterURL               string
	kubeconfig              string
	admissionListenAddress  string
	admissionTLSCertPath    string
	admissionTLSKeyPath     string
	caBundleFile            string
	useExternalBootstrap    bool
	namespace               string
	workerClusterKubeconfig string
	versionConstraint       string
}

func main() {
	nodeFlags := node.NewFlags(flag.CommandLine)
	logFlags := machinecontrollerlog.NewDefaultOptions()
	logFlags.AddFlags(flag.CommandLine)

	opt := &options{}

	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&opt.kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if flag.Lookup("master") == nil {
		flag.StringVar(&opt.masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	flag.StringVar(&opt.admissionListenAddress, "listen-address", ":9876", "The address on which the MutatingWebhook will listen on")
	flag.StringVar(&opt.admissionTLSCertPath, "tls-cert-path", "/tmp/cert/tls.crt", "The path of the TLS cert for the MutatingWebhook")
	flag.StringVar(&opt.admissionTLSKeyPath, "tls-key-path", "/tmp/cert/tls.key", "The path of the TLS key for the MutatingWebhook")
	flag.StringVar(&opt.caBundleFile, "ca-bundle", "", "path to a file containing all PEM-encoded CA certificates (will be used instead of the host's certificates if set)")
	flag.StringVar(&opt.namespace, "namespace", "kubermatic", "The namespace where the webhooks will run")
	flag.StringVar(&opt.workerClusterKubeconfig, "worker-cluster-kubeconfig", "", "Path to kubeconfig of worker/user cluster where machines and machinedeployments exist. If not specified, value from --kubeconfig or in-cluster config will be used")
	flag.StringVar(&opt.versionConstraint, "kubernetes-version-constraints", ">=0.0.0", "")

	flag.BoolVar(&opt.useExternalBootstrap, "use-external-bootstrap", true, "DEPRECATED: This flag is no-op and will have no effect since machine-controller only supports external bootstrap mechanism. This flag is only kept for backwards compatibility and will be removed in the future")

	flag.Parse()

	if err := logFlags.Validate(); err != nil {
		log.Fatalf("Invalid options: %v", err)
	}

	rawLog := machinecontrollerlog.New(logFlags.Debug, logFlags.Format)
	log := rawLog.Sugar()

	// set the logger used by controller-runtime
	ctrlruntimelog.SetLogger(zapr.NewLogger(rawLog.WithOptions(zap.AddCallerSkip(1))))

	opt.kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	opt.masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	if opt.caBundleFile != "" {
		if err := util.SetCABundleFile(opt.caBundleFile); err != nil {
			log.Fatalw("-ca-bundle is invalid", zap.Error(err))
		}
	}

	cfg, err := clientcmd.BuildConfigFromFlags(opt.masterURL, opt.kubeconfig)
	if err != nil {
		log.Fatalw("Failed to build kubeconfig", zap.Error(err))
	}

	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		log.Fatalw("Failed to build client", zap.Error(err))
	}

	constraint, err := semver.NewConstraint(opt.versionConstraint)
	if err != nil {
		log.Fatalw("Failed to validate kubernetes-version-constraints", zap.Error(err))
	}

	// Start with assuming that current cluster will be used as worker cluster
	workerClient := client
	// Handing for worker client
	if opt.workerClusterKubeconfig != "" {
		workerClusterConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: opt.workerClusterKubeconfig},
			&clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			log.Fatalw("Failed to create worker cluster config", zap.Error(err))
		}

		// Build dedicated client for worker cluster
		workerClient, err = ctrlruntimeclient.New(workerClusterConfig, ctrlruntimeclient.Options{})
		if err != nil {
			log.Fatalw("Failed to build worker client", zap.Error(err))
		}
	}

	srv, err := admission.Builder{
		ListenAddress:      opt.admissionListenAddress,
		Log:                log,
		Client:             client,
		WorkerClient:       workerClient,
		NodeFlags:          nodeFlags,
		Namespace:          opt.namespace,
		VersionConstraints: constraint,

		// we could change this to get the CertDir from the configured CertName
		// and KeyName, but doing so does not bring us any benefits but would
		// technically break compatibility.
		CertDir:  "/",
		CertName: opt.admissionTLSCertPath,
		KeyName:  opt.admissionTLSKeyPath,
	}.Build()
	if err != nil {
		log.Fatalw("Failed to create admission hook", zap.Error(err))
	}

	log.Infow("Listening", "address", opt.admissionListenAddress)

	serverContext := signals.SetupSignalHandler()
	if err := srv.Start(serverContext); err != nil {
		log.Fatalw("Failed to start server", zap.Error(err))
	}
}
