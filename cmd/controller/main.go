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
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	machineclientset "github.com/kubermatic/machine-controller/pkg/client/clientset/versioned"
	machineinformers "github.com/kubermatic/machine-controller/pkg/client/informers/externalversions"
	machinelistersv1alpha1 "github.com/kubermatic/machine-controller/pkg/client/listers/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/clusterinfo"
	"github.com/kubermatic/machine-controller/pkg/controller"
	machinehealth "github.com/kubermatic/machine-controller/pkg/health"
	"github.com/kubermatic/machine-controller/pkg/machines"
	"github.com/kubermatic/machine-controller/pkg/signals"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	masterURL     string
	kubeconfig    string
	clusterDNSIPs string
	listenAddress string
	name          string
	workerCount   int
)

const (
	controllerName                     = "machine-controller"
	defaultLeaderElectionNamespace     = "kube-system"
	defaultLeaderElectionLeaseDuration = 15 * time.Second
	defaultLeaderElectionRenewDeadline = 10 * time.Second
	defaultLeaderElectionRetryPeriod   = 2 * time.Second
)

// controllerRunOptions holds data that are required to create and run machine controller
type controllerRunOptions struct {
	// kubeClient a client that knows how to consume kubernetes API
	kubeClient *kubernetes.Clientset

	// extClient a client that knows how to consume kubernetes extension API
	extClient *apiextclient.Clientset

	// machineClient a client that knows how to consume Machine resources
	machineClient *machineclientset.Clientset

	// this essentially sets the cluster DNS IP addresses. The list is passed to kubelet and then down to pods.
	clusterDNSIPs []net.IP

	// metrics a struct that holds all metrics we want to collect
	metrics *controller.MetricsCollection

	// leaderElectionClient holds a client that is used by the leader election library
	leaderElectionClient *kubernetes.Clientset

	// nodeInformer holds a shared informer for Nodes
	nodeInformer cache.SharedIndexInformer

	// nodeLister holds a lister that knows how to list Nodes from a cache
	nodeLister listerscorev1.NodeLister

	// secreSystemNstLister knows hot to list Secrects that are inside kube-system namespace from a cache
	secretSystemNsLister listerscorev1.SecretLister

	// machineInformer holds a shared informer for Machines
	machineInformer cache.SharedIndexInformer

	// machineLister holds a lister that knows how to list Machines from a cache
	machineLister machinelistersv1alpha1.MachineLister

	// kubeconfigProvider knows how to get cluster information stored under a ConfigMap
	kubeconfigProvider controller.KubeconfigProvider

	// name of the controller. When set the controller will only process machines with the annotation "machine.k8s.io/controller": name
	name string

	// parentCtx carries a cancellation signal
	parentCtx context.Context

	// parentCtxDone allows you to close parentCtx
	// since context can form a tree-like structure it seems to be odd to pass done function of a parent
	// and allow dependant function to close the parent.
	// it should be the other way around i.e. derive a new context from the parent
	parentCtxDone context.CancelFunc
}

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&clusterDNSIPs, "cluster-dns", "10.10.10.10", "Comma-separated list of DNS server IP address.")
	flag.IntVar(&workerCount, "worker-count", 5, "Number of workers to process machines. Using a high number with a lot of machines might cause getting rate-limited from your cloud provider.")
	flag.StringVar(&listenAddress, "internal-listen-address", "127.0.0.1:8085", "The address on which the http server will listen on. The server exposes metrics on /metrics, liveness check on /live and readiness check on /ready")
	flag.StringVar(&name, "name", "", "When set, the controller will only process machines with the annotation \"machine.k8s.io/controller\": name")

	flag.Parse()

	ips, err := parseClusterDNSIPs(clusterDNSIPs)
	if err != nil {
		glog.Fatalf("invalid cluster dns specified: %v", err)
	}

	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("error building kubeconfig: %v", err)
	}

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

	err = machines.EnsureCustomResourceDefinitions(extClient)
	if err != nil {
		glog.Fatalf("failed to create CustomResourceDefinition: %v", err)
	}

	// before we acquire a lock we actually warm up caches mirroring the state of the API server
	machineInformerFactory := machineinformers.NewSharedInformerFactory(machineClient, time.Second*30)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	kubePublicKubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(kubeClient, time.Second*30, metav1.NamespacePublic, nil)
	kubeSystemInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(kubeClient, time.Second*30, metav1.NamespaceSystem, nil)
	defaultKubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(kubeClient, time.Second*30, metav1.NamespaceDefault, nil)

	kubeconfigProvider := clusterinfo.New(cfg, kubePublicKubeInformerFactory.Core().V1().ConfigMaps().Lister(), defaultKubeInformerFactory.Core().V1().Endpoints().Lister())
	runOptions := controllerRunOptions{
		kubeClient:           kubeClient,
		extClient:            extClient,
		machineClient:        machineClient,
		metrics:              controller.NewMachineControllerMetrics(),
		clusterDNSIPs:        ips,
		leaderElectionClient: leaderElectionClient,
		nodeInformer:         kubeInformerFactory.Core().V1().Nodes().Informer(),
		nodeLister:           kubeInformerFactory.Core().V1().Nodes().Lister(),
		secretSystemNsLister: kubeSystemInformerFactory.Core().V1().Secrets().Lister(),
		machineInformer:      machineInformerFactory.Machine().V1alpha1().Machines().Informer(),
		machineLister:        machineInformerFactory.Machine().V1alpha1().Machines().Lister(),
		kubeconfigProvider:   kubeconfigProvider,
		name:                 name,
	}

	kubeInformerFactory.Start(stopCh)
	kubePublicKubeInformerFactory.Start(stopCh)
	defaultKubeInformerFactory.Start(stopCh)
	machineInformerFactory.Start(stopCh)
	kubeSystemInformerFactory.Start(stopCh)

	syncsMaps := []map[reflect.Type]bool{
		kubeInformerFactory.WaitForCacheSync(stopCh),
		kubePublicKubeInformerFactory.WaitForCacheSync(stopCh),
		machineInformerFactory.WaitForCacheSync(stopCh),
		defaultKubeInformerFactory.WaitForCacheSync(stopCh),
		kubeSystemInformerFactory.WaitForCacheSync(stopCh),
	}
	for _, syncsMap := range syncsMaps {
		for key, synced := range syncsMap {
			if !synced {
				glog.Fatalf("unable to sync %s", key)
			}
		}
	}

	ctx, ctxDone := context.WithCancel(context.Background())
	var g run.Group
	{
		prometheus.MustRegister(controller.NewMachineCollector(
			machineInformerFactory.Machine().V1alpha1().Machines().Lister(),
		))

		prometheus.MustRegister(controller.NewNodeCollector(
			kubeInformerFactory.Core().V1().Nodes().Lister(),
		))

		s := createUtilHttpServer(kubeClient, kubeconfigProvider)
		g.Add(func() error {
			return s.ListenAndServe()
		}, func(err error) {
			ctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			s.Shutdown(ctx)
		})
	}
	{
		g.Add(func() error {
			select {
			case <-stopCh:
				return errors.New("user requested to stop the application")
			case <-ctx.Done():
				return errors.New("parent context has been closed - propagating the request")
			}
		}, func(err error) {
			ctxDone()
		})

	}
	{
		g.Add(func() error {
			runOptions.parentCtx = ctx
			runOptions.parentCtxDone = ctxDone
			return startControllerViaLeaderElection(runOptions)
		}, func(err error) {
			ctxDone()
		})
	}

	glog.Info(g.Run())
}

// startControllerViaLeaderElection starts machine controller only if a proper lock was acquired.
// This essentially means that we can have multiple instances and at the same time only one is operational.
// The program terminates when the leadership was lost.
func startControllerViaLeaderElection(runOptions controllerRunOptions) error {
	id, err := os.Hostname()
	if err != nil {
		glog.Fatalf("error getting hostname: %s", err.Error())
	}
	// add a seed to the id, so that two processes on the same host don't accidentally both become active
	id = id + "_" + string(uuid.NewUUID())

	rl := resourcelock.EndpointsLock{
		EndpointsMeta: metav1.ObjectMeta{
			Namespace: defaultLeaderElectionNamespace,
			Name:      controllerName,
		},
		Client: runOptions.leaderElectionClient.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      id + fmt.Sprintf("-%s", controllerName),
			EventRecorder: createRecorder(runOptions.kubeClient),
		},
	}

	// I think this might be a bit paranoid but the fact the there is no way
	// to stop the leader election library might cause synchronization issues.
	// imagine that a user wants to shutdown the app but since there is no way of telling the library to stop it will eventually run `runController` method
	// and bad things can happen - the fact it works at the moment doesn't mean it will in the future
	runController := func(_ <-chan struct{}) {
		machineController := controller.NewMachineController(
			runOptions.kubeClient,
			runOptions.machineClient,
			runOptions.nodeInformer,
			runOptions.nodeLister,
			runOptions.machineInformer,
			runOptions.machineLister,
			runOptions.secretSystemNsLister,
			runOptions.clusterDNSIPs,
			runOptions.metrics,
			runOptions.kubeconfigProvider,
			runOptions.name,
		)

		err := machineController.Run(workerCount, runOptions.parentCtx.Done())
		if err != nil {
			glog.Errorf("error running controller: %v", err)
			runOptions.parentCtxDone()
			return
		}
		glog.Info("machine controller has been successfully stopped")
	}

	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          &rl,
		LeaseDuration: defaultLeaderElectionLeaseDuration,
		RenewDeadline: defaultLeaderElectionRenewDeadline,
		RetryPeriod:   defaultLeaderElectionRetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: runController,
			OnStoppedLeading: func() {
				runOptions.parentCtxDone()
			},
		},
	})
	if err != nil {
		return err
	}
	go le.Run()

	<-runOptions.parentCtx.Done()
	return nil
}

// createUtilHttpServer creates a new HTTP server
func createUtilHttpServer(kubeClient *kubernetes.Clientset, kubeconfigProvider controller.KubeconfigProvider) *http.Server {
	health := healthcheck.NewHandler()
	health.AddReadinessCheck("apiserver-connection", machinehealth.ApiserverReachable(kubeClient))

	for name, c := range readinessChecks(kubeconfigProvider) {
		health.AddReadinessCheck(name, c)
	}

	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.Handler())
	m.Handle("/live", http.HandlerFunc(health.LiveEndpoint))
	m.Handle("/ready", http.HandlerFunc(health.ReadyEndpoint))

	return &http.Server{
		Addr:         listenAddress,
		Handler:      m,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

func readinessChecks(kubeconfigProvider controller.KubeconfigProvider) map[string]healthcheck.Check {
	return map[string]healthcheck.Check{
		"valid-info-kubeconfig": func() error {
			cm, err := kubeconfigProvider.GetKubeconfig()
			if err != nil {
				glog.V(2).Infof("[healthcheck] Unable to get kubeconfig: %v", err)
				return err
			}
			if len(cm.Clusters) != 1 {
				err := errors.New("[healthcheck] Invalid kubeconfig: no clusters found")
				glog.V(2).Info(err)
				return err
			}
			for name, c := range cm.Clusters {
				if len(c.CertificateAuthorityData) == 0 {
					err := fmt.Errorf("[healthcheck] Invalid kubeconfig: no certificate authority data was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
				if len(c.Server) == 0 {
					err := fmt.Errorf("[healthcheck] Invalid kubeconfig: no server was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
			}
			return nil
		},
	}
}

// createRecorder creates a new event recorder which is later used by the leader election
// library to broadcast events
func createRecorder(kubeClient *kubernetes.Clientset) record.EventRecorder {
	glog.V(4).Info("creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(4).Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName})
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
