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

package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	machineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	machineinformers "github.com/kubermatic/machine-controller/pkg/client/informers/externalversions"
	"github.com/kubermatic/machine-controller/pkg/controller"
	machinehealth "github.com/kubermatic/machine-controller/pkg/health"
	"github.com/kubermatic/machine-controller/pkg/machines"
	"github.com/kubermatic/machine-controller/pkg/signals"
	"github.com/kubermatic/machine-controller/pkg/ssh"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	masterURL           string
	kubeconfig          string
	sshKeyName          string
	clusterDNSIPs       string
	healthListenAddress string
	workerCount         int
)

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&sshKeyName, "ssh-key-name", "machine-controller", "The name of the private key. This name will be used when a public key will be created at the cloud provider.")
	flag.StringVar(&clusterDNSIPs, "cluster-dns", "10.10.10.10", "Comma-separated list of DNS server IP address.")
	flag.StringVar(&healthListenAddress, "health-listen-address", "127.0.0.1:8086", "Listen address for the readiness/liveness http server. The endpoints are /live /ready")
	flag.IntVar(&workerCount, "worker-count", 5, "Number of workers to process machines. Using a high number with a lot of machines might cause getting rate-limited from your cloud provider.")

	flag.Parse()

	ips, err := parseClusterDNSIPs(clusterDNSIPs)
	if err != nil {
		glog.Fatalf("invalid cluster dns specified: %v", err)
	}

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %v", err)
	}

	extclient := apiextclient.NewForConfigOrDie(cfg)
	err = machines.EnsureCustomResourceDefinitions(extclient)
	if err != nil {
		glog.Fatalf("failed to create CustomResourceDefinition: %v", err)
	}

	machineClient, err := machineclientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %v", err)
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	machineInformerFactory := machineinformers.NewSharedInformerFactory(machineClient, time.Second*30)

	key, err := ssh.EnsureSSHKeypairSecret(sshKeyName, kubeClient)
	if err != nil {
		glog.Fatalf("failed to get/create ssh key configmap: %v", err)
	}

	c := controller.NewMachineController(kubeClient, machineClient, kubeInformerFactory, machineInformerFactory, key, ips)

	go kubeInformerFactory.Start(stopCh)
	go machineInformerFactory.Start(stopCh)

	for _, syncsMap := range []map[reflect.Type]bool{kubeInformerFactory.WaitForCacheSync(stopCh), machineInformerFactory.WaitForCacheSync(stopCh)} {
		for key, synced := range syncsMap {
			if !synced {
				glog.Fatalf("unable to sync %s", key)
			}
		}
	}

	health := healthcheck.NewHandler()
	health.AddReadinessCheck("apiserver-connection", machinehealth.ApiserverReachable(kubeClient))
	health.AddReadinessCheck("custom-resource-definitions-exist", machinehealth.CustomResourceDefinitionsEstablished(extclient))
	for name, c := range c.ReadinessChecks() {
		health.AddReadinessCheck(name, c)
	}
	go http.ListenAndServe(healthListenAddress, health)

	if err = c.Run(workerCount, stopCh); err != nil {
		glog.Fatalf("Error running controller: %v", err)
	}
}

func parseClusterDNSIPs(s string) ([]net.IP, error) {
	var ips []net.IP
	sips := strings.Split(s, ",")
	for _, sip := range sips {
		ip := net.ParseIP(strings.TrimSpace(sip))
		if ip == nil {
			return nil, fmt.Errorf("unable to parse ip %s", sip)
		}
		ips = append(ips, ip)
	}
	return ips, nil
}
