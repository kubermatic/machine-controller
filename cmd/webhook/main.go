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

	"github.com/golang/glog"

	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubermatic/machine-controller/pkg/admission"
	userdatamanager "github.com/kubermatic/machine-controller/pkg/userdata/manager"
)

var (
	masterURL              string
	kubeconfig             string
	admissionListenAddress string
	admissionTLSCertPath   string
	admissionTLSKeyPath    string
)

func main() {
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if flag.Lookup("master") == nil {
		flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	flag.StringVar(&admissionListenAddress, "listen-address", ":9876", "The address on which the MutatingWebhook will listen on")
	flag.StringVar(&admissionTLSCertPath, "tls-cert-path", "/tmp/cert/cert.pem", "The path of the TLS cert for the MutatingWebhook")
	flag.StringVar(&admissionTLSKeyPath, "tls-key-path", "/tmp/cert/key.pem", "The path of the TLS key for the MutatingWebhook")
	flag.Parse()
	kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("error building kubeconfig: %v", err)
	}

	client, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		glog.Fatalf("failed to build client: %v", err)
	}

	um, err := userdatamanager.New()
	if err != nil {
		glog.Fatalf("error initialising userdata plugins: %v", err)
	}

	s := admission.New(admissionListenAddress, client, um)
	if err := s.ListenAndServeTLS(admissionTLSCertPath, admissionTLSKeyPath); err != nil {
		glog.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			glog.Fatalf("Failed to shutdown server: %v", err)
		}
	}()
	glog.Infof("Listening on %s", admissionListenAddress)
	select {}
}
