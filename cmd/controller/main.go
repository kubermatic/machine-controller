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
	"strings"
	"time"

	"os"

	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	machineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	machineinformers "github.com/kubermatic/machine-controller/pkg/client/informers/externalversions"
	"github.com/kubermatic/machine-controller/pkg/controller"
	machinehealth "github.com/kubermatic/machine-controller/pkg/health"
	"github.com/kubermatic/machine-controller/pkg/machines"
	"github.com/kubermatic/machine-controller/pkg/ssh"
	"github.com/kubermatic/machine-controller/pkg/utils"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

var (
	masterURL     string
	kubeconfig    string
	sshKeyName    string
	clusterDNSIPs string
	listenAddress string
	workerCount   int
)

const (
	controllerName = "machine-controller"

	defaultLeaderElectionNamespace     = "kube-system"
	defaultLeaderElectionLeaseDuration = 15 * time.Second
	defaultLeaderElectionRenewDeadline = 10 * time.Second
	defaultLeaderElectionRetryPeriod   = 2 * time.Second
)

// TODO:desc
type controllerContext struct {
	// TODO: desc
	kubeClient kubernetes.Interface

	// TODO: desc
	extClient apiextclient.Interface

	// TODO: desc
	machineClient machineclientset.Interface
	// TODO add leaderElectionClient

	// TODO: desc
	sshKeyPair *ssh.PrivateKey

	// TODO: desc
	ips []net.IP

	// TODO: desc
	metrics *MachineControllerMetrics

	// TODO: desc
	kubeInformerFactory kubeinformers.SharedInformerFactory

	// TODO: desc
	machineInformerFactory machineinformers.SharedInformerFactory

	// TODO: desc
	stopCh <-chan struct{}
}

// TODO: desc
func createClientsOrDie(cfg *rest.Config) (kubernetes.Interface, apiextclient.Interface, machineclientset.Interface, kubernetes.Interface) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building kubernetes clientset for kubeClient: %v", err)
	}

	extClient, err := apiextclient.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building kubernetes clientset for extClient: %v", err)
	}

	machineClient, err := machineclientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building example clientset for machineClient: %v", err)
	}

	leaderElectionClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building kubernetes clientset for leaderElectionClient: %v", err)
	}

	return kubeClient, extClient, machineClient, leaderElectionClient
}

// TODO: desc
func newControllerContextFromExisting(ctx controllerContext) controllerContext {
	ctx.kubeInformerFactory = kubeinformers.NewSharedInformerFactory(ctx.kubeClient, time.Second*30)
	ctx.machineInformerFactory = machineinformers.NewSharedInformerFactory(ctx.machineClient, time.Second*30)
	return ctx
}

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&sshKeyName, "ssh-key-name", "machine-controller", "The name of the private key. This name will be used when a public key will be created at the cloud provider.")
	flag.StringVar(&clusterDNSIPs, "cluster-dns", "10.10.10.10", "Comma-separated list of DNS server IP address.")
	flag.IntVar(&workerCount, "worker-count", 5, "Number of workers to process machines. Using a high number with a lot of machines might cause getting rate-limited from your cloud provider.")
	flag.StringVar(&listenAddress, "internal-listen-address", "127.0.0.1:8085", "The address on which the http server will listen on. The server exposes metrics on /metrics, liveness check on /live and readiness check on /ready")

	flag.Parse()

	ips, err := parseClusterDNSIPs(clusterDNSIPs)
	if err != nil {
		glog.Fatalf("invalid cluster dns specified: %v", err)
	}

	// set up signals so we handle the first shutdown signal gracefully
	// TODO: fix stopCh
	//stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("error building kubeconfig: %v", err)
	}

	kubeClient, extClient, machineClient, leaderElectionClient := createClientsOrDie(cfg)

	err = machines.EnsureCustomResourceDefinitions(extClient)
	if err != nil {
		glog.Fatalf("failed to create CustomResourceDefinition: %v", err)
	}

	key, err := ssh.EnsureSSHKeypairSecret(sshKeyName, kubeClient)
	if err != nil {
		glog.Fatalf("failed to get/create ssh key configmap: %v", err)
	}

	startUtilHttpServer(kubeClient)

	metrics := NewMachineControllerMetrics()
	run := func(stopCh <-chan struct{}) {
		ctx := newControllerContextFromExisting(controllerContext{
			kubeClient:    kubeClient,
			extClient:     extClient,
			machineClient: machineClient,
			sshKeyPair:    key,
			metrics:       metrics,
			ips:           ips,
			stopCh:        stopCh,
		})

		c := controller.NewMachineControllerOrDie(ctx.kubeClient,
			ctx.machineClient,
			ctx.kubeInformerFactory,
			ctx.machineInformerFactory,
			ctx.sshKeyPair,
			ctx.ips,
			controller.MetricsCollection{
				Machines:            metrics.Machines,
				Workers:             metrics.Workers,
				Errors:              metrics.Errors,
				Nodes:               metrics.Nodes,
				ControllerOperation: metrics.ControllerOperation,
				NodeJoinDuration:    metrics.NodeJoinDuration,
			},
			stopCh)

		if err = c.Run(workerCount, stopCh); err != nil {
			glog.Fatalf("error running controller: %v", err)
		}
	}

	startControllerViaLeaderElectionOrDie(leaderElectionClient, createRecorder(kubeClient), run)
}

// TODO: desc
func createConfigMapInformer(kubeClient kubernetes.Interface) listerscorev1.ConfigMapLister {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	configMapInformer := kubeInformerFactory.Core().V1().ConfigMaps()
	return configMapInformer.Lister()
}

// TODO: desc
func createRecorder(kubeClient kubernetes.Interface) record.EventRecorder {
	glog.V(4).Info("creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(4).Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	return eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: controllerName})
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

// TODO: desc
func startControllerViaLeaderElectionOrDie(leaderElectionClient kubernetes.Interface, recorder record.EventRecorder, run func(<-chan struct{})) {
	id, err := os.Hostname()
	if err != nil {
		glog.Fatalf("error getting hostname: %s", err.Error())
	}
	// TODO: id + UUID ??!!

	rl := resourcelock.EndpointsLock{
		EndpointsMeta: metav1.ObjectMeta{
			Namespace: defaultLeaderElectionNamespace,
			Name:      controllerName,
		},
		Client: leaderElectionClient.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      id + fmt.Sprintf("-external-%s", controllerName),
			EventRecorder: recorder,
		},
	}

	leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
		Lock:          &rl,
		LeaseDuration: defaultLeaderElectionLeaseDuration,
		RenewDeadline: defaultLeaderElectionRenewDeadline,
		RetryPeriod:   defaultLeaderElectionRetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				glog.Infof("leaderelection lost")
			},
		},
	})
}

func startUtilHttpServer(kubeClient kubernetes.Interface) {
	health := healthcheck.NewHandler()
	health.AddReadinessCheck("apiserver-connection", machinehealth.ApiserverReachable(kubeClient))
	for name, c := range utils.ReadinessChecks(createConfigMapInformer(kubeClient)) {
		health.AddReadinessCheck(name, c)
	}

	go serveUtilHttpServer(health)
}

func serveUtilHttpServer(health healthcheck.Handler) {
	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.Handler())
	m.Handle("/live", http.HandlerFunc(health.LiveEndpoint))
	m.Handle("/ready", http.HandlerFunc(health.ReadyEndpoint))

	s := http.Server{
		Addr:    listenAddress,
		Handler: m,
	}
	glog.V(4).Infof("serving util http server on %s", listenAddress)
	glog.Fatalf("util http server died: %v", s.ListenAndServe())
}
